package chat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
	"winone/internal/settings"
)

const PreviewTextLimit = 10 << 20

// New-page commands create an empty file exclusively. Resolve the real parent
// first so a junction cannot escape the workspace, and never replace a file.
func (s *Store) CreatePreviewFile(id, path string) (FilePreview, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return FilePreview{}, err
	}
	root, err := existingDirectory(c.Directory)
	if err != nil {
		return FilePreview{}, err
	}
	if strings.TrimSpace(path) == "" {
		return FilePreview{}, errors.New("请输入文件名")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = workspaceAlias(root, path)
	if err != nil {
		return FilePreview{}, err
	}
	parent, err := resolvePreviewPath(filepath.Dir(path))
	if err != nil {
		return FilePreview{}, err
	}
	if !within(root, parent) {
		return FilePreview{}, errors.New("文件位置不在当前工作文件夹内")
	}
	resolved := filepath.Join(parent, filepath.Base(path))
	file, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return FilePreview{}, err
	}
	if err = file.Close(); err != nil {
		return FilePreview{}, err
	}
	return s.PreviewFile(id, path)
}

type FilePreview struct {
	Path         string `json:"path"`
	ResolvedPath string `json:"resolvedPath,omitempty"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Modified     int64  `json:"modified"`
	Kind         string `json:"kind"`
	MIME         string `json:"mimeType"`
	Revision     string `json:"revision"`
	URL          string `json:"url,omitempty"`
	ReadOnly     bool   `json:"readOnly,omitempty"`
}

// Resolve an explicitly addressed workspace alias. External targets are allowed
// only through that alias, while imported attachments retain their own boundary.
func (s *Store) ResolvePreviewFile(id, path string) (string, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return "", err
	}
	root, err := existingDirectory(c.Directory)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	resolved, err := resolvePreviewPath(path)
	if err != nil {
		return "", err
	}
	// Imported attachment copies may live outside the workspace. Reuse the
	// attachment validator rather than granting access to the whole history.
	if !within(root, filepath.Clean(path)) && validateConversationAttachments(c, []string{path}) != nil && !s.resourcePreviewAllowed(id, path, resolved) && !s.reviewPreviewAllowed(id, path, resolved) {
		return "", errors.New("文件位置不在当前工作文件夹内")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("只能预览普通文件")
	}
	return resolved, nil
}

func (s *Store) PreviewFile(id, path string) (FilePreview, error) {
	resolved, err := s.ResolvePreviewFile(id, path)
	if err != nil {
		return FilePreview{}, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return FilePreview{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return FilePreview{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, PreviewTextLimit+1))
	if err != nil {
		return FilePreview{}, err
	}
	extension := strings.ToLower(filepath.Ext(resolved))
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		mimeType = httpContentType(data)
	}
	// Windows may associate TypeScript with MPEG transport streams. The source
	// viewer must use readable file contents before that ambiguous association.
	if extension == ".ts" && utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
		mimeType = "text/plain; charset=utf-8"
	}
	c, err := s.Conversation(id)
	if err != nil {
		return FilePreview{}, err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Directory, path)
	}
	path = filepath.Clean(path)
	p := FilePreview{Path: path, Name: filepath.Base(path), Size: info.Size(), Modified: info.ModTime().UnixMilli(), Kind: "binary", MIME: mimeType, ReadOnly: !within(c.Directory, resolved) || within(filepath.Join(c.SessionDirectory, "attachments"), resolved)}
	if !strings.EqualFold(path, resolved) {
		p.ResolvedPath = resolved
	}
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		p.Kind = "image"
	case strings.HasPrefix(mimeType, "video/"):
		p.Kind = "video"
	case strings.HasPrefix(mimeType, "audio/"):
		p.Kind = "audio"
	case strings.HasPrefix(mimeType, "application/pdf"):
		p.Kind = "pdf"
	case int64(len(data)) == info.Size() && utf8.Valid(data) && !bytes.ContainsRune(data, 0):
		p.Kind = "text"
		digest := sha256.Sum256(data)
		p.Revision = hex.EncodeToString(digest[:])
	case info.Size() > PreviewTextLimit:
		p.Kind = "oversize"
	}
	return p, nil
}

func httpContentType(data []byte) string {
	if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

// Editing is optimistic: a changed file requires a fresh read. Atomic replace
// avoids leaving a partly written document if a write or flush fails.
func (s *Store) SavePreviewFile(id, path, revision, text string) (FilePreview, error) {
	if len(text) > PreviewTextLimit || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return FilePreview{}, errors.New("文件不是可编辑的 UTF-8 文本，或超过 10 MB")
	}
	before, err := s.PreviewFile(id, path)
	if err != nil {
		return before, err
	}
	if before.ReadOnly {
		return before, errors.New("工作区外的链接目标和会话附件副本为只读，请修改原文件")
	}
	if before.Kind != "text" || revision == "" || before.Revision != revision {
		return before, errors.New("磁盘文件已更改，请重新加载后再保存")
	}
	resolved, err := s.ResolvePreviewFile(id, path)
	if err != nil {
		return before, err
	}
	expected := before.Path
	if before.ResolvedPath != "" {
		expected = before.ResolvedPath
	}
	if !strings.EqualFold(resolved, expected) {
		return before, errors.New("链接目标已更改，请重新加载后再保存")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return before, err
	}
	if err = settings.WriteAtomic(resolved, []byte(text)); err != nil {
		return before, err
	}
	if err = os.Chmod(resolved, info.Mode().Perm()); err != nil {
		return before, err
	}
	return s.PreviewFile(id, path)
}

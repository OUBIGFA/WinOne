package chat

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/settings"
)

const attachmentLimit = 20 << 20

func attachmentByteLimit(data []byte) int {
	switch http.DetectContentType(data) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return MediaImageLimit
	}
	return attachmentLimit
}

// MaximumChatRequestBytes is the complete serialized model request budget.
// Pi scales transmitted image copies to this limit while retaining originals.
const MaximumChatRequestBytes = 16 << 20

type Attachment struct {
	Name            string          `json:"name"`
	Path            string          `json:"path"`
	Size            int64           `json:"size"`
	MimeType        string          `json:"mimeType"`
	Annotation      json.RawMessage `json:"annotation,omitempty"`
	ImageAnnotation json.RawMessage `json:"imageAnnotation,omitempty"`
}

// A native file choice imports an immutable request copy. The original stays
// where the user selected it and can be edited without changing a sent message.
func (s *Store) AddAttachments(id string, paths []string) ([]Attachment, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 || len(paths) > 16 {
		return nil, errors.New("一次请选择 1 至 16 个附件")
	}
	result := []Attachment{}
	for _, source := range paths {
		if !filepath.IsAbs(source) {
			return nil, errors.New("附件位置必须是绝对路径")
		}
		info, err := os.Stat(source)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > MediaImageLimit {
			return nil, errors.New("图片不得超过 100 MB，其他附件不得超过 20 MB")
		}
		reader, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, MediaImageLimit+1))
		reader.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > attachmentByteLimit(data) {
			return nil, errors.New("图片不得超过 100 MB，其他附件不得超过 20 MB")
		}
		file, err := importAttachment(c, filepath.Base(source), data)
		if err != nil {
			return nil, err
		}
		result = append(result, file)
	}
	return result, nil
}

// ImportAttachment accepts bytes from a single-use upload grant. The caller
// selects only a display filename, never a destination or another session's path.
func (s *Store) ImportAttachment(id, name string, data []byte) (Attachment, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return Attachment{}, err
	}
	return importAttachment(c, name, data)
}

func importAttachment(c Conversation, name string, data []byte) (Attachment, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\\/:\x00") || strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return Attachment{}, errors.New("附件名称无效")
	}
	if len(data) > attachmentByteLimit(data) {
		return Attachment{}, errors.New("图片不得超过 100 MB，其他附件不得超过 20 MB")
	}
	target := filepath.Join(c.SessionDirectory, "attachments", rand.Text(), name)
	if err := settings.WriteAtomic(target, data); err != nil {
		return Attachment{}, err
	}
	return Attachment{Name: name, Path: target, Size: int64(len(data)), MimeType: http.DetectContentType(data)}, nil
}

func validateConversationAttachments(c Conversation, paths []string) error {
	if len(paths) > 16 {
		return errors.New("每条消息最多 16 个附件")
	}
	for _, file := range paths {
		if !within(filepath.Join(c.SessionDirectory, "attachments"), file) {
			return errors.New("附件未导入当前会话")
		}
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			return err
		}
		if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(file)) {
			return errors.New("附件路径不能经过链接")
		}
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("附件类型无效")
		}
		var header [512]byte
		reader, err := os.Open(file)
		if err != nil {
			return err
		}
		n, readErr := io.ReadFull(reader, header[:])
		reader.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if info.Size() > int64(attachmentByteLimit(header[:n])) {
			return errors.New("附件类型或大小无效")
		}
	}
	return nil
}
func (s *Store) ValidateAttachments(id string, paths []string) error {
	c, err := s.Conversation(id)
	if err != nil {
		return err
	}
	return validateConversationAttachments(c, paths)
}
func (s *Store) AttachmentDetails(id string, paths []string) ([]Attachment, error) {
	if err := s.ValidateAttachments(id, paths); err != nil {
		return nil, err
	}
	result := []Attachment{}
	for _, file := range paths {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		result = append(result, Attachment{Name: filepath.Base(file), Path: file, Size: int64(len(data)), MimeType: http.DetectContentType(data)})
	}
	return result, nil
}

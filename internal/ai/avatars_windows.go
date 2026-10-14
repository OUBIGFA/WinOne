//go:build windows

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AvatarInput struct {
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

// ImportAvatar delegates storage, HTTPS fetching and favicon discovery to the
// pinned Magpie implementation, in the shared WinOne AI data directory.
func (t Translator) ImportAvatar(ctx context.Context, operation string, input AvatarInput) (string, error) {
	if operation != "icons.store" && operation != "icons.fetch" && operation != "icons.website" {
		return "", errors.New("不支持的头像操作")
	}
	if len(input.Data) > MaxAvatarBytes || len(input.URL) > 4096 || len(input.Name) > 256 {
		return "", errors.New("头像参数过大")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	data, err := t.invoke(ctx, map[string]any{"operation": operation, "icon": input})
	var response struct {
		Icon  string `json:"icon"`
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &response) != nil {
		return "", errors.New("头像服务无法启动，请修复安装")
	}
	if response.Error != "" {
		return "", errors.New(response.Error)
	}
	if err != nil || !IsStoredAvatar(response.Icon) {
		return "", errors.New("无法保存头像，请检查图片后重试")
	}
	return response.Icon, nil
}

// ReadAvatar never starts an AI process. The content hash and confined root
// also prevent a forged reference or a replaced file from reaching the UI.
func (t Translator) ReadAvatar(icon string, offset int) (AvatarChunk, error) {
	fail := func() (AvatarChunk, error) {
		return AvatarChunk{}, errors.New("头像无法读取，请重新选择图片")
	}
	if !IsStoredAvatar(icon) || offset < 0 || offset >= MaxAvatarBytes {
		return fail()
	}
	root, err := os.OpenRoot(filepath.Join(t.DataDirectory, "config", "magpie", "icons"))
	if err != nil {
		return fail()
	}
	defer root.Close()
	name := strings.TrimPrefix(icon, "file:")
	file, err := root.Open(name)
	if err != nil {
		return fail()
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxAvatarBytes {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxAvatarBytes+1))
	if err != nil || len(data) > MaxAvatarBytes || offset >= len(data) {
		return fail()
	}
	sum := sha256.Sum256(data)
	if !strings.HasPrefix(name, hex.EncodeToString(sum[:8])+".") {
		return fail()
	}
	mime := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".ico": "image/x-icon", ".svg": "image/svg+xml"}[filepath.Ext(name)]
	return AvatarChunk{Data: data[offset:min(offset+AvatarChunkBytes, len(data))], Size: len(data), MimeType: mime}, nil
}

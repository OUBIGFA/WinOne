package chat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/settings"
)

// Match Cindy's byte-action budget; model requests still use their separate
// serialized request budget and resize transmitted image copies.
const MediaImageLimit = 100 << 20

// Image copies use immutable conversation storage and never alter the source.
func (s *Store) ImportMedia(id, name string, data []byte) (Attachment, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return Attachment{}, err
	}
	if len(data) == 0 || len(data) > MediaImageLimit {
		return Attachment{}, errors.New("图片必须是 100 MB 以内的文件")
	}
	kind := http.DetectContentType(data)
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "image/bmp": ".bmp", "image/x-icon": ".ico", "image/vnd.microsoft.icon": ".ico"}[kind]
	if ext == "" && (strings.Contains(kind, "text/") || strings.Contains(kind, "xml")) {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if start, ok := token.(xml.StartElement); ok {
				if start.Name.Local == "svg" && (start.Name.Space == "" || start.Name.Space == "http://www.w3.org/2000/svg") {
					ext = ".svg"
					kind = "image/svg+xml"
				}
				break
			}
		}
	}
	if ext == "" {
		return Attachment{}, errors.New("内容不是可查看的图片")
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || strings.ContainsAny(name, "\\/:\x00") {
		name = "image"
	}
	name = strings.TrimSuffix(name, filepath.Ext(name)) + ext
	path := filepath.Join(c.SessionDirectory, "attachments", rand.Text(), name)
	if err = settings.WriteAtomic(path, data); err != nil {
		return Attachment{}, err
	}
	return Attachment{Name: name, Path: path, Size: int64(len(data)), MimeType: kind}, nil
}

func (s *Store) ImportRemoteMedia(ctx context.Context, id, rawURL string) (Attachment, error) {
	if _, err := s.Conversation(id); err != nil {
		return Attachment{}, err
	}
	valid := func(raw string) (*url.URL, error) {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("图片地址必须使用 HTTP 或 HTTPS，且不能包含登录信息")
		}
		return u, nil
	}
	address, err := valid(rawURL)
	if err != nil {
		return Attachment{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return Attachment{}, err
	}
	client := &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("图片地址重定向过多")
		}
		_, err := valid(request.URL.String())
		return err
	}}
	response, err := client.Do(request)
	if err != nil {
		var failed *url.Error
		if errors.As(err, &failed) {
			err = failed.Err
		}
		return Attachment{}, fmt.Errorf("读取图片失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Attachment{}, fmt.Errorf("图片服务返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MediaImageLimit+1))
	if err != nil {
		return Attachment{}, err
	}
	return s.ImportMedia(id, filepath.Base(address.Path), data)
}

// An inactive conversation receives only the new attachments. Its saved text
// and pending input are read and retained under the same draft write lock.
func (s *Store) AppendMediaDraft(id string, files []Attachment, replace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft, err := s.draftLocked(id)
	if err != nil {
		return err
	}
	for _, file := range files {
		found := false
		for i, old := range draft.Attachments {
			if old.Path == file.Path {
				found = true
				break
			}
			if replace != "" && old.Path == replace {
				draft.Attachments[i] = file
				found = true
				break
			}
		}
		if !found {
			draft.Attachments = append(draft.Attachments, file)
		}
	}
	return s.saveDraftLocked(id, draft)
}

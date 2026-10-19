//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"path/filepath"
	"strings"
	"winone/internal/chat"
)

// Both sides come from the selected source. An old revision never silently
// falls back to the current file, which would misrepresent the reviewed change.
func (c *chatControl) reviewFilePreview(ctx context.Context, id, turn, scope, base, path, side, revision string) (any, error) {
	params, _ := json.Marshal(map[string]string{"sessionId": id, "turnId": turn, "scope": scope, "base": base})
	snapshot, err := c.reviewAction(ctx, "review.snapshot", params)
	if err != nil {
		return nil, err
	}
	selected := snapshot.(chat.ReviewSnapshot)
	if revision != "" && selected.Revision != revision {
		return nil, errors.New("文件已变化，请刷新差异后重试")
	}
	read := func(ctx context.Context) ([]byte, bool, error) {
		if selected.Source == "session" {
			return selected.RecordedFileVersion(path, side)
		}
		return chat.ReadReviewVersion(ctx, selected, path, side)
	}
	data, present, err := read(ctx)
	if err != nil {
		return nil, err
	}
	if !present {
		return map[string]any{"present": false}, nil
	}
	size := len(data)
	data = nil
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	server, err := c.previewServer()
	if err != nil {
		return nil, err
	}
	address, err := server.OpenContent(id, filepath.Base(path), contentType, func(ctx context.Context) ([]byte, error) {
		current, err := c.reviewAction(ctx, "review.snapshot", params)
		if err != nil {
			return nil, err
		}
		if current.(chat.ReviewSnapshot).Revision != selected.Revision {
			return nil, errors.New("文件已变化，请刷新差异后重试")
		}
		data, present, err := read(ctx)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, errors.New("文件版本已不可用")
		}
		return data, nil
	})
	return map[string]any{"present": true, "url": address, "size": size, "mime": contentType, "root": selected.Root}, err
}

//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// RequestArchive uses Magpie's own name validation and body preview limits.
// Full body bytes are available only to the native export path below.
func (t Translator) RequestArchive(ctx context.Context, id string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, err := t.invokeBounded(ctx, map[string]any{"operation": "archive.read", "archive": id}, 4<<20)
	if err != nil {
		return nil, errors.New("请求归档无法读取，文件可能已超过容量限制而被清理，或保存失败")
	}
	var result struct {
		Bytes *int64 `json:"bytes"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Bytes == nil {
		return nil, errors.New("请求归档响应无效")
	}
	// Escaped JSON can exceed the desktop's 1 MiB frame even when each body
	// fits Magpie's text preview. Offer the full file instead of failing IPC.
	if len(raw) > 768<<10 {
		return json.Marshal(map[string]any{"bytes": *result.Bytes, "large": true})
	}
	return raw, nil
}

// CopyRequestArchive streams the original JSON without the desktop's frame
// limit. The caller publishes its temporary file only after this succeeds.
func (t Translator) CopyRequestArchive(ctx context.Context, id string, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	process, err := t.start(ctx)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(process.input).Encode(map[string]any{"operation": "archive.file", "archive": id}); err != nil {
		process.closeJob()
		process.finish()
		return err
	}
	process.input.Close()
	_, copyErr := io.Copy(output, process.output)
	if copyErr != nil {
		process.closeJob()
	}
	exitErr := process.finish()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if copyErr != nil {
		return copyErr
	}
	if exitErr != nil {
		return errors.New("请求归档导出失败，原文件可能已被清理")
	}
	return nil
}

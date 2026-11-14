//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (t Translator) ImportProviderSetup(ctx context.Context, text string) (json.RawMessage, error) {
	if len(text) > 1<<20 {
		return nil, errors.New("连接说明过长")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := t.invoke(ctx, map[string]any{"operation": "models.import", "text": text})
	if err != nil {
		return nil, errors.New("无法读取连接说明，请检查内容后重试")
	}
	return output, nil
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// The session is fixed by its private bridge. Imported credentials and processes
// stay in the OneBot worker; only the redacted MCP catalog/results cross to Pi.
func (b *chatOneBot) importedTools(ctx context.Context, sessionID, operation string, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		CallID string          `json:"callId"`
		Name   string          `json:"name"`
		Args   json.RawMessage `json:"args"`
	}
	if operation == "call" {
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if input.CallID == "" || len(input.CallID) > 256 || input.Name == "" || len(input.Name) > 128 {
			return nil, errors.New("导入工具参数无效")
		}
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"sessionId": sessionID, "callId": input.CallID, "name": input.Name, "args": input.Args}
	// Cancelling the Pi tool must reach the MCP server's AbortSignal, not merely
	// stop waiting on stdout while an imported shell/network operation continues.
	stop := context.AfterFunc(ctx, func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = runner.Call(cancelCtx, "importedTools.cancel", params)
	})
	defer stop()
	return runner.Call(ctx, "importedTools."+operation, params)
}

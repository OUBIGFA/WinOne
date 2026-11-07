//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Native WebView identities are attached to the authenticated pipe peer. The
// HTML frame, a model argument and a caller-supplied ownerPid cannot select the
// process whose documents are created or released.
func (g *chatGhosts) callUI(ctx context.Context, method string, params json.RawMessage, peer uint32) (json.RawMessage, error) {
	if method == "connection-submit" || method == "answer" {
		var input struct {
			ID     string
			Answer struct {
				Action string
				Value  json.RawMessage
			}
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		g.mu.Lock()
		prompt := g.prompts[input.ID]
		g.mu.Unlock()
		var request struct{ Kind string }
		if prompt != nil {
			_ = json.Unmarshal(prompt.Request, &request)
		}
		if method == "connection-submit" {
			if peer == 0 || request.Kind != "plugin-setup" || input.Answer.Action != "submit" {
				return nil, errors.New("凭证提交必须来自当前原生配置框")
			}
			return g.call(ctx, "answer", params)
		}
		if request.Kind == "plugin-setup" && len(input.Answer.Value) > 0 {
			return nil, errors.New("凭证不能通过普通插件回答通道提交")
		}
	}
	if method == "mcp-app-native" || method == "mcp-app-release-owner" {
		var fields map[string]json.RawMessage
		if peer == 0 || json.Unmarshal(params, &fields) != nil {
			return nil, errors.New("原生插件页来源无效")
		}
		if method == "mcp-app-native" {
			var caller string
			if json.Unmarshal(fields["caller"], &caller) != nil || !strings.HasPrefix(caller, "native:"+strconv.FormatUint(uint64(peer), 10)+":") {
				return nil, errors.New("插件页调用者不属于当前原生进程")
			}
		}
		fields["ownerPid"], _ = json.Marshal(peer)
		var err error
		params, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	}
	return g.call(ctx, method, params)
}

func (g *chatGhosts) releaseUIOwner(ctx context.Context, peer uint32) {
	g.mu.Lock()
	runner := g.runner
	g.mu.Unlock()
	if runner != nil {
		_, _ = runner.Call(ctx, "mcp-app-release-owner", map[string]uint32{"ownerPid": peer})
	}
}

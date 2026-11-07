//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"winone/internal/chat"
)

// The existing Pi committed-generation admission hook also serves plugin
// tasks. Its bridge captures the conversation; a model cannot change the
// plugin/task identity or supply somebody else's execution receipt.
func (g *chatGhosts) taskAdmissionBridge(conversation chat.Conversation) (*chat.PrivateBridge, error) {
	return chat.NewPrivateBridge(g.control.ctx, g.taskAdmissionHandler(conversation))
}

func (g *chatGhosts) taskAdmissionHandler(conversation chat.Conversation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || (r.URL.Path != "/v1/admit" && r.URL.Path != "/v1/refs" && r.URL.Path != "/v1/ghost-input-admit" && r.URL.Path != "/v1/ghost-review") {
			http.NotFound(w, r)
			return
		}
		var input struct {
			RequestID string          `json:"requestId"`
			Execution json.RawMessage `json:"execution"`
			URLs      []string        `json:"urls,omitempty"`
			Request   json.RawMessage `json:"request,omitempty"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&input)
		if err == nil && r.URL.Path != "/v1/refs" && (input.RequestID == "" || len(input.RequestID) > 128) {
			err = errors.New("插件任务输入身份无效")
		}
		if err == nil && r.URL.Path == "/v1/refs" && len(input.URLs) > 32 {
			err = errors.New("插件任务 PR 引用过多")
		}
		if err == nil && (r.URL.Path == "/v1/ghost-input-admit" || r.URL.Path == "/v1/ghost-review") && !strings.HasPrefix(input.RequestID, "plugin-target:") {
			err = errors.New("插件目标输入身份无效")
		}
		var result json.RawMessage
		if err == nil {
			runner, startErr := g.ensure(r.Context())
			err = startErr
			if err == nil {
				if r.URL.Path == "/v1/refs" {
					result, err = runner.Call(r.Context(), "tasks-refs", map[string]any{"sessionId": conversation.ID, "urls": input.URLs})
				} else if r.URL.Path == "/v1/ghost-review" {
					result, err = runner.Call(r.Context(), "targets-review", map[string]any{"sessionId": conversation.ID, "requestId": input.RequestID, "execution": input.Execution, "request": input.Request})
				} else {
					method := "tasks-admit"
					if strings.HasPrefix(input.RequestID, "plugin-target:") {
						method = "targets-admit"
					}
					result, err = runner.Call(r.Context(), method, map[string]any{"sessionId": conversation.ID, "requestId": input.RequestID, "execution": input.Execution})
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		_, _ = w.Write(result)
	}
}

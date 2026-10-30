//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"winone/internal/chat"
)

// Task admission runs at Pi's committed generation boundary, including queue
// drain and durable recovery. The private bridge binds the task identity; model
// tool arguments cannot nominate a different task or provide owner authority.
func (b *chatOneBot) taskAdmissionBridge(conversation chat.Conversation) (*chat.PrivateBridge, error) {
	return chat.NewPrivateBridge(b.control.ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || (r.URL.Path != "/v1/admit" && r.URL.Path != "/v1/refs") {
			http.NotFound(w, r)
			return
		}
		var input struct {
			RequestID string          `json:"requestId"`
			Execution json.RawMessage `json:"execution"`
			URLs      []string        `json:"urls,omitempty"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&input)
		if err == nil && r.URL.Path == "/v1/admit" && (input.RequestID == "" || len(input.RequestID) > 128) {
			err = errors.New("任务输入身份无效")
		}
		var result json.RawMessage
		if err == nil {
			runner, startErr := b.ensure(r.Context())
			err = startErr
			if err == nil {
				if r.URL.Path == "/v1/refs" {
					result, err = runner.Call(r.Context(), "tasks.pr.record", map[string]any{"childSessionId": conversation.ID, "urls": input.URLs})
				} else {
					result, err = runner.Call(r.Context(), "tasks.accept", map[string]any{"delegationId": conversation.OneBotTaskID, "childSessionId": conversation.ID, "requestId": input.RequestID, "execution": input.Execution})
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
	}))
}

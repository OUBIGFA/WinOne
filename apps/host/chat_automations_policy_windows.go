//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"winone/internal/configjson"
)

// This route is inside the conversation's authenticated AI bridge. Neither
// a browser nor the model supplies a different conversation or run identity.
func (c *chatControl) automationPolicy(w http.ResponseWriter, r *http.Request, id string) bool {
	if !strings.HasSuffix(r.URL.Path, "/scheduler/policy") {
		return false
	}
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return true
	}
	var input struct {
		Action string `json:"action"`
	}
	raw := json.RawMessage{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&raw); err != nil {
		http.Error(w, err.Error(), 400)
		return true
	}
	if err := configjson.DecodeObject(raw, &input); err != nil || input.Action != "silence" && input.Action != "notify" {
		http.Error(w, "Invalid run policy", 400)
		return true
	}
	requestID := c.engine.ActiveRequest(id)
	if requestID == "" || c.automations == nil {
		http.Error(w, "No active automation turn", 409)
		return true
	}
	runner, err := c.automations.ensure(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return true
	}
	value, err := runner.Call(r.Context(), "policy", map[string]string{"sessionId": id, "requestId": requestID, "action": input.Action})
	if err != nil {
		http.Error(w, err.Error(), 409)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(value)
	return true
}

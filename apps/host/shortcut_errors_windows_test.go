//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"testing"
	"winone/internal/host"
)

func TestShortcutErrorsStayWithTheirAction(t *testing.T) {
	runtime := &shortcutRuntime{}
	runtime.report("capture", errors.New("capture failed"))
	runtime.report("pin", errors.New("pin failed"))
	runtime.report("capture", nil)
	var snapshot struct {
		ActionErrors map[string]string `json:"actionErrors"`
	}
	if err := json.Unmarshal(runtime.handle(host.Request{ID: "test", Method: "snapshot"}).Result, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ActionErrors["pin"] != "pin failed" || snapshot.ActionErrors["capture"] != "" {
		t.Fatal(snapshot)
	}
}

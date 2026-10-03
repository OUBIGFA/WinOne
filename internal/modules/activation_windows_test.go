//go:build windows

package modules

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/host"
)

func TestCaptureActivationDoesNotStartAnIdleWorker(t *testing.T) {
	module := NewLazy(context.Background(), "capture", "Capture", filepath.Join(t.TempDir(), "missing.exe"), "unused", t.TempDir())
	reply := module.Handle(host.Request{Version: 1, ID: "activate", Method: "capture.activate"})
	var result struct {
		Active bool `json:"active"`
	}
	if reply.Error != nil || json.Unmarshal(reply.Result, &result) != nil || result.Active || module.process != nil {
		t.Fatalf("activation started an idle worker: %+v", reply)
	}
}

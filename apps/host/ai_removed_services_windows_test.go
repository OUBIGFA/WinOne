//go:build windows

package main

import (
	"context"
	"os"
	"testing"
	"winone/internal/host"
)

func TestRemovedAIServicesHaveNoRPCOrWorkerLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	directory := t.TempDir()
	service := newAIService(ctx, directory, "no-installed-worker.exe")
	for _, method := range []string{"routing.open", "routing.close", "usage.list", "usage.export.save", "archive.read", "archive.export.save", "privacy.get", "plugins.list", "sessions.list", "library.list", "otel.get"} {
		response := service.handle(host.Request{ID: method, Method: method})
		if response.Error == nil || response.Error.Message != "不支持的 AI 操作" {
			t.Errorf("removed operation still has a handler: %s: %+v", method, response)
		}
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 0 || service.backend.process != nil || service.store != nil {
		t.Fatal("a removed service accessed configuration or started a worker", err)
	}
}

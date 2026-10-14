//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/settings"
)

func TestBrokenAISettingsDoNotStopOtherHostFeatures(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	if err = os.WriteFile(filepath.Join(directory, "ai-settings.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = registerAI(service, context.Background(), directory, filepath.Join(directory, "WinOne.Host.exe")); err != nil {
		t.Fatal(err)
	}
	if response := service.Handle(host.Request{Version: 1, ID: "snapshot", Method: "snapshot"}); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := service.Handle(host.Request{Version: 1, ID: "ai", Method: "ai.settings.get"}); response.Error == nil {
		t.Fatal("invalid AI settings reported success")
	}
}

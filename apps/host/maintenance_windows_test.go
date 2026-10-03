//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"winone/internal/host"
)

type maintenanceWorker struct {
	methods    []string
	reply      string
	closed     bool
	closeError error
}

func (w *maintenanceWorker) Resume() {}
func (w *maintenanceWorker) Handle(r host.Request) host.Response {
	w.methods = append(w.methods, r.Method)
	return host.Response{ID: r.ID, Result: json.RawMessage(w.reply)}
}
func (w *maintenanceWorker) Close() error { w.closed = true; return w.closeError }

func TestUninstallWorkerRequiresCompletedRecoveryBeforeClose(t *testing.T) {
	for _, reply := range []string{`{"state":"failed","error":"restore failed"}`, `{"state":"running"}`, `{}`, `invalid`} {
		w := &maintenanceWorker{reply: reply}
		if err := releaseWinTray(context.Background(), w, "uninstall.prepare"); err == nil || w.closed {
			t.Fatalf("invalid result accepted: %s", reply)
		}
	}
	w := &maintenanceWorker{reply: `{"state":"completed"}`, closeError: errors.New("worker still running")}
	if err := releaseWinTray(context.Background(), w, "uninstall.prepare"); err == nil || !w.closed {
		t.Fatal("close failure hidden")
	}
	if w.methods[0] != "uninstall.prepare" {
		t.Fatal(w.methods)
	}
}

func TestMaintenanceRecognizesAllBundledProcesses(t *testing.T) {
	for _, name := range []string{"WinOne.exe", "WinOne.Host.exe", "WinOne.WinTray.exe", "WinOne.Capture.exe", "WinOne.AlwaysOnTop.exe", "WINONE.EXE"} {
		if !isWinOneProcess(name) {
			t.Fatal(name)
		}
	}
	if isWinOneProcess("notepad.exe") || isWinOneProcess("WinOneSetup.exe") {
		t.Fatal("unrelated process blocked")
	}
}

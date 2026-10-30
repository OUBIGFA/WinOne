//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"winone/internal/host"
)

func TestArchiveDesktopHelper(t *testing.T) {
	if os.Getenv("WINONE_ARCHIVE_DESKTOP_HELPER") != "1" {
		return
	}
	io.Copy(io.Discard, os.Stdin)
}

// A real native-peer process must own the export. The complete archive stays
// between Worker, Host and disk; only a compact receipt reaches the desktop.
func TestPackagedArchiveExportRequiresLiveDesktopAndPreservesFailedDestination(t *testing.T) {
	worker := os.Getenv("WINONE_AI_TEST_EXE")
	if worker == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	root := t.TempDir()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	desktop := filepath.Join(root, "WinOne.exe")
	if err := os.WriteFile(desktop, binary, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	peer := exec.CommandContext(ctx, desktop, "-test.run=^TestArchiveDesktopHelper$")
	peer.Env = append(os.Environ(), "WINONE_ARCHIVE_DESKTOP_HELPER=1")
	peer.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := peer.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); peer.Wait() }()
	s := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	s.translator.Executable = worker
	id := "2026-10-26/101010-fixture"
	archive := filepath.Join(s.translator.DataDirectory, "config", "magpie", "request-archive", "2026-10-26", "101010-fixture.json")
	if err := os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": id, "request": map[string]any{"body": strings.Repeat("body fixture", 200000)}, "response": map[string]any{"body": "reply"}})
	if err := os.WriteFile(archive, body, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "selected.json")
	save := func(id string) host.Response {
		params, _ := json.Marshal(map[string]string{"archive": id, "path": destination})
		return s.handle(host.Request{ID: "save", Method: "archive.export.save", PeerPID: uint32(peer.Process.Pid), Params: params})
	}
	result := save(id)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	var receipt struct{ State, Path string }
	if json.Unmarshal(result.Result, &receipt) != nil || receipt.State != "saved" || receipt.Path != destination || len(result.Result) > 1024 {
		t.Fatal("invalid export receipt")
	}
	saved, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(saved, body) {
		t.Fatal("exported JSON was truncated", err)
	}
	if result := save("2026-10-26/missing"); result.Error == nil {
		t.Fatal("missing archive reported saved")
	}
	saved, err = os.ReadFile(destination)
	if err != nil || !bytes.Equal(saved, body) {
		t.Fatal("failed export changed existing destination", err)
	}
	input.Close()
	if err := peer.Wait(); err != nil {
		t.Fatal(err)
	}
	if result := save(id); result.Error == nil {
		t.Fatal("closed native owner was allowed to export")
	}
}

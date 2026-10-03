//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDiagnosticsReadsBoundedLogsWithoutHost(t *testing.T) {
	directory := t.TempDir()
	module := filepath.Join(directory, "modules", "wintray")
	if err := os.MkdirAll(module, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "wintray.log"), []byte(strings.Repeat("运行", 20000)+"\nlatest failure"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "worker.log"), []byte("worker output"), 0600); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(filepath.Join(module, "worker.log"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	app := &App{dataDirectory: directory}
	logs, err := app.ReadModuleLogs("wintray")
	if err != nil {
		t.Fatal(err)
	}
	if !logs.FolderAvailable || len(logs.Entries) != 2 {
		t.Fatalf("missing logs: %+v", logs)
	}
	entry := logs.Entries[0]
	if !entry.Available || !entry.Truncated || len(entry.Text) > 64<<10 || !utf8.ValidString(entry.Text) || !strings.HasSuffix(entry.Text, "latest failure") {
		t.Fatal("tail was not bounded and readable")
	}
	if logs.Entries[1].Text != "worker output" {
		t.Fatal("missing process output")
	}
}

func TestDiagnosticsMissingLogsDoNotCreateModuleDirectory(t *testing.T) {
	directory := t.TempDir()
	logs, err := (&App{dataDirectory: directory}).ReadModuleLogs("capture")
	if err != nil {
		t.Fatal(err)
	}
	if logs.FolderAvailable || len(logs.Entries) != 1 || logs.Entries[0].Available {
		t.Fatal("missing log reported available")
	}
	if _, err = os.Stat(filepath.Join(directory, "modules")); !os.IsNotExist(err) {
		t.Fatal("diagnostics created module data")
	}
}

func TestHostDiagnosticsNeverMixesModuleLogs(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "logs", "host.log"), []byte("host only"), 0600); err != nil {
		t.Fatal(err)
	}
	logs, err := (&App{dataDirectory: directory}).ReadModuleLogs("host")
	if err != nil || len(logs.Entries) != 1 || logs.Entries[0].Text != "host only" {
		t.Fatal(logs, err)
	}
}

func TestDiagnosticsRejectsPathsAndNonFiles(t *testing.T) {
	directory := t.TempDir()
	app := &App{dataDirectory: directory}
	for _, id := range []string{"../capture", "C:\\private", "wintray/worker.log", "unknown"} {
		if _, err := app.ReadModuleLogs(id); err == nil {
			t.Fatalf("accepted arbitrary target %q", id)
		}
	}
	if err := os.MkdirAll(filepath.Join(directory, "modules", "capture", "worker.log"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReadModuleLogs("capture"); err == nil {
		t.Fatal("log directory treated as a file")
	}
}

func TestDiagnosticsRejectsLogSymlinkOutsideDataDirectory(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	module := filepath.Join(directory, "modules", "capture")
	if err := os.MkdirAll(module, 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "private.txt")
	if err := os.WriteFile(secret, []byte("not a module log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(module, "worker.log")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := (&App{dataDirectory: directory}).ReadModuleLogs("capture"); err == nil {
		t.Fatal("external symlink was readable")
	}
}

//go:build windows

package desktop

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// readWithin fails the test when a read does not return in time, as a hung
// Shell call would leave the page busy for good.
func readWithin(t *testing.T, directory, path string) (ProgramMetadata, error) {
	t.Helper()
	type reply struct {
		info ProgramMetadata
		err  error
	}
	done := make(chan reply, 1)
	go func() {
		info, err := ProgramInfo(directory, path)
		done <- reply{info, err}
	}()
	select {
	case r := <-done:
		return r.info, r.err
	case <-time.After(20 * time.Second):
		t.Fatalf("reading %s did not return", path)
		return ProgramMetadata{}, nil
	}
}

func TestProgramInfoReadsTheDescriptionAndIconAndCachesThem(t *testing.T) {
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if _, err := os.Stat(notepad); err != nil {
		t.Skip("no notepad.exe")
	}
	directory := t.TempDir()
	info, err := readWithin(t, directory, notepad)
	if err != nil || info.Name == "" || info.Name == "notepad" {
		t.Fatalf("info %q, %v", info.Name, err)
	}
	icon, err := png.Decode(bytes.NewReader(info.Icon))
	if err != nil || icon.Bounds().Dx() != 32 || icon.Bounds().Dy() != 32 {
		t.Fatalf("icon %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(directory, "cache", "program-icons"))
	if len(entries) != 1 {
		t.Fatalf("cache %v", entries)
	}
}

func TestProgramInfoNamesAScriptByItsFileAndRefusesAFolder(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "backup.ps1")
	if err := os.WriteFile(script, []byte("# never run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := readWithin(t, "", script)
	if err != nil || info.Name != "backup" {
		t.Fatalf("script %q, %v", info.Name, err)
	}
	if _, err := readWithin(t, "", directory); err == nil {
		t.Fatal("a folder was read as a program")
	}
}

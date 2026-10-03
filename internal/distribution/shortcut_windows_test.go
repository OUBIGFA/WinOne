//go:build windows

package distribution

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRealShortcutBackupPreservesFile(t *testing.T) {
	dir, archive := installedFixture(t)
	shortcut := filepath.Join(t.TempDir(), "WinOne.lnk")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", `$ErrorActionPreference='Stop'; $w=New-Object -ComObject WScript.Shell; $s=$w.CreateShortcut($env:WINONE_SHORTCUT_PATH); $s.TargetPath=$env:WINONE_SHORTCUT_TARGET; $s.Arguments='--headless'; $s.WorkingDirectory=Split-Path -Parent $env:WINONE_SHORTCUT_TARGET; $s.Save(); [void][Runtime.InteropServices.Marshal]::ReleaseComObject($s); [void][Runtime.InteropServices.Marshal]::ReleaseComObject($w)`)
	cmd.Env = append(os.Environ(), "WINONE_SHORTCUT_PATH="+shortcut, "WINONE_SHORTCUT_TARGET="+filepath.Join(dir, "WinOne.exe"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shortcut fixture: %v %s", err, output)
	}
	before, err := os.ReadFile(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(dir, archive, backup); err != nil {
		t.Fatal(err)
	}
	if err := BackupShortcut(archive, backup, shortcut); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(backup, "shortcut.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ManifestSHA256 string           `json:"manifestSha256"`
		Shortcut       ShortcutSnapshot `json:"shortcut"`
	}
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.ManifestSHA256) != 64 || !bytes.Equal(saved.Shortcut.Data, before) {
		t.Fatal("shortcut bytes or binding lost")
	}
	if err = ValidateShortcutSnapshot(saved.Shortcut); err != nil {
		t.Fatal(err)
	}
	if err = BackupShortcut(archive, backup, shortcut); err == nil {
		t.Fatal("snapshot overwritten")
	}
	after, err := os.ReadFile(shortcut)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source shortcut changed", err)
	}
	replacement := exec.Command(cmd.Path, cmd.Args[1:]...)
	replacement.Env = append(os.Environ(), "WINONE_SHORTCUT_PATH="+shortcut, "WINONE_SHORTCUT_TARGET="+filepath.Join(dir, "WinOne.Host.exe"))
	if output, err := replacement.CombinedOutput(); err != nil {
		t.Fatalf("replacement shortcut: %v %s", err, output)
	}
	written, err := CaptureShortcut(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	if written.SHA256 == saved.Shortcut.SHA256 {
		t.Fatal("fixture did not change shortcut")
	}
	if err = RestoreShortcut(shortcut, saved.Shortcut, written); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(shortcut)
	if err != nil || !bytes.Equal(before, restored) {
		t.Fatal("real shortcut bytes were not restored", err)
	}
}

func TestShortcutSnapshotRejectsParentJunction(t *testing.T) {
	outside := t.TempDir()
	data := make([]byte, 76)
	copy(data, shellLinkHeader)
	if err := os.WriteFile(filepath.Join(outside, "WinOne.lnk"), data, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "linked")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", "New-Item -ItemType Junction -Path $env:WINONE_TEST_JUNCTION -Target $env:WINONE_TEST_OUTSIDE -ErrorAction Stop | Out-Null")
	cmd.Env = append(os.Environ(), "WINONE_TEST_JUNCTION="+alias, "WINONE_TEST_OUTSIDE="+outside)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("junction: %v %s", err, output)
	}
	if _, err := CaptureShortcut(filepath.Join(alias, "WinOne.lnk")); err == nil {
		t.Fatal("shortcut junction accepted")
	}
}

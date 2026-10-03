package distribution

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func shortcutState(t *testing.T, path string, tag byte) ShortcutSnapshot {
	t.Helper()
	data := make([]byte, 80)
	copy(data, shellLinkHeader)
	data[79] = tag
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := CaptureShortcut(path)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestShortcutRestoreChecksCurrentStateAndCanRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WinOne.lnk")
	original := shortcutState(t, path, 1)
	written := shortcutState(t, path, 2)
	if err := RestoreShortcut(path, original, written); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, original.Data) {
		t.Fatal("original shortcut not restored", err)
	}
	if err := RestoreShortcut(path, original, written); err != nil {
		t.Fatal("retry failed", err)
	}
	external := shortcutState(t, path, 3)
	if err := RestoreShortcut(path, original, written); err == nil {
		t.Fatal("external change overwritten")
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, external.Data) {
		t.Fatal("external shortcut changed", err)
	}
	if err := RestoreShortcut(filepath.Join(t.TempDir(), "Other.lnk"), original, written); err == nil {
		t.Fatal("snapshot redirected to another path")
	}
}

func TestShortcutRestorePreservesOriginalAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WinOne.lnk")
	original, err := CaptureShortcut(path)
	if err != nil {
		t.Fatal(err)
	}
	written := shortcutState(t, path, 2)
	if err = RestoreShortcut(path, original, written); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("new shortcut retained", err)
	}
	if err = RestoreShortcut(path, original, written); err != nil {
		t.Fatal(err)
	}
}

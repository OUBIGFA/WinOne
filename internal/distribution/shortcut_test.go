package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestShortcutSnapshotPreservesBytesAndAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WinOne.lnk")
	missing, err := CaptureShortcut(path)
	if err != nil || missing.Exists {
		t.Fatal(missing, err)
	}
	data := make([]byte, 80)
	copy(data, []byte{0x4c, 0, 0, 0, 1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46})
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureShortcut(path)
	if err != nil || !snapshot.Exists || !bytes.Equal(snapshot.Data, data) {
		t.Fatal(snapshot, err)
	}
	sum := sha256.Sum256(data)
	if snapshot.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("shortcut digest missing")
	}
	if err = ValidateShortcutSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Data[40] ^= 1
	if err = ValidateShortcutSnapshot(snapshot); err == nil {
		t.Fatal("changed snapshot accepted")
	}
}

func TestShortcutSnapshotRejectsInvalidFiles(t *testing.T) {
	for _, data := range [][]byte{[]byte("not a shortcut"), make([]byte, (1<<20)+1)} {
		path := filepath.Join(t.TempDir(), "WinOne.lnk")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := CaptureShortcut(path); err == nil {
			t.Fatal("invalid shortcut accepted")
		}
	}
}

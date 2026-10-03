package distribution

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// RestoreShortcut restores only the explicitly named destination. Both the
// original and updater-written states must be validated snapshots. Callers hold
// the maintenance lease; state comparisons detect observed external edits but
// are not an OS-level compare-and-swap against arbitrary concurrent writers.
func RestoreShortcut(destination string, original, expectedCurrent ShortcutSnapshot) error {
	for _, snapshot := range []ShortcutSnapshot{original, expectedCurrent} {
		if err := ValidateShortcutSnapshot(snapshot); err != nil {
			return err
		}
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, snapshot := range []ShortcutSnapshot{original, expectedCurrent} {
		relative, err := filepath.Rel(snapshot.Path, destination)
		if err != nil || relative != "." {
			return errors.New("shortcut snapshot destination mismatch")
		}
	}
	same := func(a, b ShortcutSnapshot) bool { return a.Exists == b.Exists && a.SHA256 == b.SHA256 }
	current, err := CaptureShortcut(destination)
	if err != nil {
		return err
	}
	if same(current, original) {
		return nil
	} // Recovery may resume after a successful write.
	if !same(current, expectedCurrent) {
		return errors.New("shortcut changed outside the upgrade")
	}
	parentPath := filepath.Dir(destination)
	info, err := os.Lstat(parentPath)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("shortcut restore parent must be a real directory")
	}
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(destination)
	if original.Exists {
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return err
		}
		temporary := ".winone-shortcut-" + hex.EncodeToString(random[:]) + ".tmp"
		file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer root.Remove(temporary)
		_, writeErr := file.Write(original.Data)
		if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return err
		}
		current, err = CaptureShortcut(destination)
		if err != nil {
			return err
		}
		if !same(current, expectedCurrent) {
			return errors.New("shortcut changed while preparing restoration")
		}
		if err = root.Rename(temporary, name); err != nil {
			return err
		}
	} else {
		if err = root.Remove(name); err != nil {
			return err
		}
	}
	restored, err := CaptureShortcut(destination)
	if err != nil {
		return err
	}
	if !same(restored, original) {
		return errors.New("shortcut restoration verification failed")
	}
	return nil
}

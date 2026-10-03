package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

// ApplyUpgradeShortcut applies a prevalidated, persisted target snapshot under
// the coordinator's maintenance lease and pending-operation marker.
func ApplyUpgradeShortcut(destination string, original, next ShortcutSnapshot, journal *UpgradeJournalStore) error {
	return executeUpgradeShortcut(destination, original, next, journal, false)
}

// RestoreUpgradeShortcut records rollback intent before restoring the original.
func RestoreUpgradeShortcut(destination string, original, next ShortcutSnapshot, journal *UpgradeJournalStore) error {
	return executeUpgradeShortcut(destination, original, next, journal, true)
}

func executeUpgradeShortcut(destination string, original, next ShortcutSnapshot, journal *UpgradeJournalStore, restore bool) error {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, snapshot := range []ShortcutSnapshot{original, next} {
		if err = ValidateShortcutSnapshot(snapshot); err != nil {
			return err
		}
		if relative, err := filepath.Rel(snapshot.Path, destination); err != nil || relative != "." {
			return errors.New("upgrade shortcut destination mismatch")
		}
	}
	same := func(a, b ShortcutSnapshot) bool { return a.Exists == b.Exists && a.SHA256 == b.SHA256 }
	inspect := func() (metadataObservation, error) {
		current, err := CaptureShortcut(destination)
		if err != nil {
			return metadataObservation{}, err
		}
		return metadataObservation{before: same(current, original), after: same(current, next)}, nil
	}
	desired, prior := next, original
	if restore {
		desired, prior = original, next
	}
	return runMetadataOperation(journal, "shortcut", shortcutOperationFingerprint(original, next), same(original, next), restore, inspect, func() error { return RestoreShortcut(destination, desired, prior) })
}
func shortcutOperationFingerprint(original, next ShortcutSnapshot) string {
	data, _ := json.Marshal([]ShortcutSnapshot{original, next})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

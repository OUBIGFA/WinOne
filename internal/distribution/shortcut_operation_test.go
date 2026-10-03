package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeShortcutJournalRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WinOne.lnk")
	original := shortcutState(t, path, 1)
	next := shortcutState(t, path, 2)
	if err := os.WriteFile(path, original.Data, 0600); err != nil {
		t.Fatal(err)
	}
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeShortcut(path, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if journal.Snapshot().MetadataStates["shortcut"] != "applied" {
		t.Fatal("shortcut not recorded")
	}
	otherOriginal := shortcutState(t, filepath.Join(t.TempDir(), "WinOne.lnk"), 3)
	otherOriginal.Path = path
	if err = ApplyUpgradeShortcut(path, otherOriginal, next, journal); err == nil {
		t.Fatal("bound original snapshot replaced")
	}
	if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeShortcut(path, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if journal.Snapshot().MetadataStates["shortcut"] != "restored" {
		t.Fatal("shortcut not restored")
	}
	actual, err := CaptureShortcut(path)
	if err != nil || actual.SHA256 != original.SHA256 {
		t.Fatal(actual, err)
	}
}

func TestUpgradeShortcutResumesIntentAndRejectsExternalEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WinOne.lnk")
	original := shortcutState(t, path, 1)
	next := shortcutState(t, path, 2)
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeShortcut(path, original, next, journal); err == nil {
		t.Fatal("new state accepted without intent")
	}
	if err = journal.BindMetadata(journal.Snapshot().Revision, "shortcut", shortcutOperationFingerprint(original, next)); err != nil {
		t.Fatal(err)
	}
	if err = journal.RecordMetadata(journal.Snapshot().Revision, "shortcut", "writing"); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenUpgradeJournal(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeShortcut(path, original, next, reopened); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Transition(reopened.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	external := shortcutState(t, path, 3)
	if err = RestoreUpgradeShortcut(path, original, next, reopened); err == nil {
		t.Fatal("external change ignored")
	}
	actual, err := CaptureShortcut(path)
	if err != nil || actual.SHA256 != external.SHA256 {
		t.Fatal("external file changed", err)
	}
}

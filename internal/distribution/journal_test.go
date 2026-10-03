package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func journalPlan() UpgradePlan {
	return UpgradePlan{PreviousVersion: "1.0.0", NextVersion: "1.0.1", PreviousManifestSHA256: strings.Repeat("a", 64), NextManifestSHA256: strings.Repeat("b", 64), Files: []UpgradeFile{{Path: "WinOne.exe", PreviousPath: "WinOne.exe", Action: "replace", PreviousSHA256: strings.Repeat("a", 64), NextSHA256: strings.Repeat("b", 64)}}}
}

func TestUpgradeJournalPersistsIntentAndEnforcesOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.json")
	store, err := CreateUpgradeJournal(path, journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(0, "committed"); err == nil {
		t.Fatal("premature commit")
	}
	if err = store.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordMetadata(1, "manifest", "unchanged"); err == nil {
		t.Fatal("version manifest update skipped")
	}
	if err = store.RecordFile(1, 0, "applied"); err == nil {
		t.Fatal("write without intent")
	}
	if err = store.RecordFile(1, 0, "writing"); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenUpgradeJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().FileStates[0] != "writing" {
		t.Fatal("intent was not durable")
	}
	if err = reopened.RecordFile(1, 0, "applied"); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err = reopened.Transition(2, "checking"); err == nil {
		t.Fatal("unfinished write accepted")
	}
	if err = reopened.RecordFile(2, 0, "applied"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Transition(3, "checking"); err == nil {
		t.Fatal("metadata skipped")
	}
	for _, name := range []string{"manifest", "uninstaller", "registration", "shortcut"} {
		if err = reopened.RecordMetadata(reopened.Snapshot().Revision, name, "writing"); err != nil {
			t.Fatal(err)
		}
		if err = reopened.RecordMetadata(reopened.Snapshot().Revision, name, "applied"); err != nil {
			t.Fatal(err)
		}
	}
	if err = reopened.Transition(reopened.Snapshot().Revision, "checking"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Transition(reopened.Snapshot().Revision, "committed"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Transition(reopened.Snapshot().Revision, "restoring"); err == nil {
		t.Fatal("completed transaction reused")
	}
}

func TestUpgradeJournalRejectsCorruptionAndStaleWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.json")
	first, err := CreateUpgradeJournal(path, journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenUpgradeJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = second.Transition(0, "applying"); err == nil {
		t.Fatal("stale writer accepted")
	}
	state := first.Snapshot()
	state.Plan.Files[0].Path = "../WinOne.exe"
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenUpgradeJournal(path); err == nil {
		t.Fatal("unsafe journal path accepted")
	}
}

func TestUpgradeJournalRequiresMetadataRollback(t *testing.T) {
	store, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordMetadata(1, "registration", "writing"); err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(2, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(3, "rolled-back"); err == nil {
		t.Fatal("uncertain registry write skipped")
	}
	if err = store.RecordMetadata(3, "registration", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordMetadata(4, "registration", "restored"); err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(5, "rolled-back"); err != nil {
		t.Fatal(err)
	}
	copy := store.Snapshot()
	copy.MetadataStates["registration"] = "writing"
	if store.Snapshot().MetadataStates["registration"] != "restored" {
		t.Fatal("metadata aliases state")
	}
}

func TestUpgradeJournalRecoversUncertainWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.json")
	store, err := CreateUpgradeJournal(path, journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{func() error { return store.Transition(0, "applying") }, func() error { return store.RecordFile(1, 0, "writing") }, func() error { return store.Transition(2, "restoring") }} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.Transition(3, "rolled-back"); err == nil {
		t.Fatal("uncertain write skipped")
	}
	if err = store.RecordFile(3, 0, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordFile(4, 0, "restored"); err != nil {
		t.Fatal(err)
	}
	if err = store.Transition(5, "rolled-back"); err != nil {
		t.Fatal(err)
	}
	copy := store.Snapshot()
	copy.FileStates[0] = "writing"
	if store.Snapshot().FileStates[0] != "restored" {
		t.Fatal("snapshot aliases state")
	}
}

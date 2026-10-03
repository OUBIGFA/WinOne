package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func fileOperationFixture(t *testing.T) (string, string, string, *UpgradeJournalStore) {
	t.Helper()
	installation, staged, backup := t.TempDir(), t.TempDir(), t.TempDir()
	for _, entry := range []struct{ root, text string }{{installation, "old"}, {staged, "new"}, {backup, "old"}} {
		if err := os.WriteFile(filepath.Join(entry.root, "WinOne.exe"), []byte(entry.text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(installation, ".winone-pending"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	plan := journalPlan()
	old, next := sha256.Sum256([]byte("old")), sha256.Sum256([]byte("new"))
	plan.Files[0].PreviousSHA256 = hex.EncodeToString(old[:])
	plan.Files[0].NextSHA256 = hex.EncodeToString(next[:])
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	return installation, staged, backup, journal
}

func TestUpgradeFileApplyAndRollbackUseJournal(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	if err := ApplyUpgradeFile(installation, staged, journal, 0); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(installation, "WinOne.exe")); err != nil || string(data) != "new" {
		t.Fatal(string(data), err)
	}
	if journal.Snapshot().FileStates[0] != "applied" {
		t.Fatal("write not recorded")
	}
	if err := ApplyUpgradeFile(installation, staged, journal, 0); err != nil {
		t.Fatal("apply retry", err)
	}
	if err := journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreUpgradeFile(installation, backup, journal, 0); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(installation, "WinOne.exe")); err != nil || string(data) != "old" {
		t.Fatal(string(data), err)
	}
	if journal.Snapshot().FileStates[0] != "restored" {
		t.Fatal("restore not recorded")
	}
}

func TestUpgradeFileRejectsExternalChangeAndCorruptSource(t *testing.T) {
	for _, scenario := range []string{"target", "source"} {
		t.Run(scenario, func(t *testing.T) {
			installation, staged, _, journal := fileOperationFixture(t)
			path := filepath.Join(installation, "WinOne.exe")
			if scenario == "source" {
				path = filepath.Join(staged, "WinOne.exe")
			}
			if err := os.WriteFile(path, []byte("other"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := ApplyUpgradeFile(installation, staged, journal, 0); err == nil {
				t.Fatal("unexpected content accepted")
			}
			want := "old"
			if scenario == "target" {
				want = "other"
			}
			if data, err := os.ReadFile(filepath.Join(installation, "WinOne.exe")); err != nil || string(data) != want {
				t.Fatal("target changed", err)
			}
		})
	}
}

func TestUpgradeFileRecoversAfterUnrecordedSuccessfulWrite(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	if err := journal.RecordFile(journal.Snapshot().Revision, 0, "writing"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installation, "WinOne.exe"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenUpgradeJournal(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeFile(installation, staged, reopened, 0); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Transition(reopened.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.RecordFile(reopened.Snapshot().Revision, 0, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(installation, "WinOne.exe"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err = OpenUpgradeJournal(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeFile(installation, backup, reopened, 0); err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().FileStates[0] != "restored" {
		t.Fatal("restore completion lost")
	}
}

func TestUpgradeFileAddsAndRemovesWithRollback(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		t.Run(action, func(t *testing.T) {
			installation, staged, backup, initial := fileOperationFixture(t)
			plan := initial.Snapshot().Plan
			plan.Files[0].Action = action
			if action == "add" {
				plan.Files[0].PreviousPath = ""
				plan.Files[0].PreviousSHA256 = ""
				if err := os.Remove(filepath.Join(installation, "WinOne.exe")); err != nil {
					t.Fatal(err)
				}
			} else {
				plan.Files[0].NextSHA256 = ""
			}
			journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "journal.json"), plan)
			if err != nil {
				t.Fatal(err)
			}
			if err = journal.Transition(0, "applying"); err != nil {
				t.Fatal(err)
			}
			if err = ApplyUpgradeFile(installation, staged, journal, 0); err != nil {
				t.Fatal(err)
			}
			if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
				t.Fatal(err)
			}
			if err = RestoreUpgradeFile(installation, backup, journal, 0); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(installation, "WinOne.exe"))
			if action == "add" {
				if !os.IsNotExist(err) {
					t.Fatal("added file retained", err)
				}
			} else if err != nil || string(data) != "old" {
				t.Fatal("removed file not restored", err)
			}
		})
	}
}

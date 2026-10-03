package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradePayloadAndManifestRoundTrip(t *testing.T) {
	t.Run("normal", func(t *testing.T) { runPayloadManifestRoundTrip(t, false) })
	t.Run("unrecorded-completion", func(t *testing.T) { runPayloadManifestRoundTrip(t, true) })
}

func runPayloadManifestRoundTrip(t *testing.T, interrupted bool) {
	installation, oldPackage := installedFixture(t)
	root, license := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "WinOne.exe"), []byte("new version bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecordBuild(root, license, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	nextPackage := filepath.Join(t.TempDir(), "next.zip")
	if err := Build(root, license, nextPackage, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	staged := extractTestPackage(t, nextPackage)
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(installation, oldPackage, backup); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpgrade(oldPackage, nextPackage)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(installation, ".winone-pending"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeManifest(installation, staged, journal); err == nil {
		t.Fatal("manifest advanced before files")
	}
	for i := range plan.Files {
		if err = ApplyUpgradeFile(installation, staged, journal, i); err != nil {
			t.Fatal(err)
		}
	}
	if interrupted {
		if err = journal.RecordMetadata(journal.Snapshot().Revision, "manifest", "writing"); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(staged, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(installation, "manifest.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		journal, err = OpenUpgradeJournal(journal.path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = ApplyUpgradeManifest(installation, staged, journal); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyUpgradePayload(installation, nextPackage); err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(journal.Snapshot().Revision, "checking"); err == nil {
		t.Fatal("installer metadata skipped")
	}
	if _, err = VerifyInstalled(installation, nextPackage); err == nil {
		t.Fatal("pending upgrade exposed as complete installation")
	}
	if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeManifest(installation, backup, journal); err == nil {
		t.Fatal("old manifest restored before files")
	}
	for i := len(plan.Files) - 1; i >= 0; i-- {
		if err = RestoreUpgradeFile(installation, backup, journal, i); err != nil {
			t.Fatal(err)
		}
	}
	if interrupted {
		if err = journal.RecordMetadata(journal.Snapshot().Revision, "manifest", "restoring"); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(backup, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(installation, "manifest.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		journal, err = OpenUpgradeJournal(journal.path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = RestoreUpgradeManifest(installation, backup, journal); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyUpgradePayload(installation, oldPackage); err != nil {
		t.Fatal(err)
	}
}

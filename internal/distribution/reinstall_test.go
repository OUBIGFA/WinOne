package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalInstallerReplacesSameVersionAndRejectsDowngrade(t *testing.T) {
	root, license := fixture(t)
	build := func(name, version string) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), name)
		if err := RecordBuild(root, license, version); err != nil {
			t.Fatal(err)
		}
		if err := Build(root, license, file, version); err != nil {
			t.Fatal(err)
		}
		return file
	}
	previous := build("old.zip", "0.0.1")
	if err := os.WriteFile(filepath.Join(root, "WinOne.exe"), []byte("fixed executable"), 0600); err != nil {
		t.Fatal(err)
	}
	next := build("next.zip", "0.0.1")
	for _, target := range []string{previous, next} {
		plan, err := PlanInstallation(previous, target)
		if err != nil {
			t.Fatalf("same-version install rejected: %v", err)
		}
		if !plan.Reinstall {
			t.Fatal("reinstall intent missing")
		}
		if _, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "journal.json"), plan); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := PlanInstallation(previous, build("older.zip", "0.0.0")); err == nil {
		t.Fatal("downgrade accepted")
	}
	if _, err := PlanUpgrade(previous, next); err == nil {
		t.Fatal("automatic updates accepted equal version")
	}
}

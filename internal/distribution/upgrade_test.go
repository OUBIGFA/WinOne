package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradePlanRequiresNewerVerifiedPackage(t *testing.T) {
	root, license := fixture(t)
	build := func(destination, version string) error {
		if err := RecordBuild(root, license, version); err != nil {
			return err
		}
		return Build(root, license, destination, version)
	}
	previous := filepath.Join(t.TempDir(), "old.zip")
	if err := build(previous, "1.0.0-alpha.2"); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.0.0-alpha.1", "1.0.0-alpha.2", "invalid"} {
		if _, err := CompareVersions("1.0.0-alpha.2", version); version == "invalid" && err == nil {
			t.Fatal("invalid version accepted")
		}
		if version == "invalid" {
			continue
		}
		next := filepath.Join(t.TempDir(), "next.zip")
		if err := build(next, version); err != nil {
			t.Fatal(err)
		}
		if _, err := PlanUpgrade(previous, next); err == nil {
			t.Fatal("not-newer package accepted", version)
		}
	}
	next := filepath.Join(t.TempDir(), "next.zip")
	if err := os.WriteFile(filepath.Join(root, "WinOne.exe"), []byte("new runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := build(next, "1.0.0-alpha.10"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpgrade(previous, next)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, change := range plan.Files {
		if change.Path == "WinOne.exe" {
			found = change.Action == "replace" && change.PreviousSHA256 != "" && change.NextSHA256 != "" && change.PreviousSHA256 != change.NextSHA256
		}
	}
	if !found {
		t.Fatal("changed executable not planned for backup/replacement")
	}
	if err := os.WriteFile(next, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanUpgrade(previous, next); err == nil {
		t.Fatal("damaged archive accepted")
	}
}

func TestVersionPrecedenceUsesSemanticOrdering(t *testing.T) {
	ordered := []string{"1.0.0-alpha.2", "1.0.0-alpha.10", "1.0.0-beta", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.10.0", "2.0.0"}
	for i := 1; i < len(ordered); i++ {
		if n, err := CompareVersions(ordered[i-1], ordered[i]); err != nil || n >= 0 {
			t.Fatal(ordered[i-1], ordered[i], n, err)
		}
	}
	for _, invalid := range []string{"1.0", "v1.0.0", "1.0.0-01", "1.0.0+metadata", ""} {
		if _, err := CompareVersions("1.0.0", invalid); err == nil {
			t.Fatal("outside package version contract", invalid)
		}
	}
}

func TestUpgradePlanTracksAddRemoveAndCaseChanges(t *testing.T) {
	root, license := fixture(t)
	for _, name := range []string{"obsolete.dll", "Case.dll"} {
		if err := os.WriteFile(filepath.Join(root, "capture", name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(t.TempDir(), "old.zip")
	if err := RecordBuild(root, license, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := Build(root, license, old, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "capture", "obsolete.dll")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "capture", "Case.dll"), filepath.Join(root, "capture", "case.dll")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capture", "added.dll"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(t.TempDir(), "next.zip")
	if err := RecordBuild(root, license, "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := Build(root, license, next, "1.0.1"); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpgrade(old, next)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"capture/obsolete.dll": "remove", "capture/added.dll": "add", "capture/case.dll": "replace", "WinOne.Host.exe": "unchanged"}
	for _, file := range plan.Files {
		if action, ok := want[file.Path]; ok {
			if file.Action != action {
				t.Fatal(file)
			}
			delete(want, file.Path)
		}
		if file.Path == "capture/case.dll" && file.PreviousPath != "capture/Case.dll" {
			t.Fatal("old spelling lost", file)
		}
	}
	if len(want) != 0 {
		t.Fatal("missing changes", want)
	}
	if plan.PreviousManifestSHA256 == plan.NextManifestSHA256 || len(plan.PreviousManifestSHA256) != 64 {
		t.Fatal("inventory binding missing")
	}
}

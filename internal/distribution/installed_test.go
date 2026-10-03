package distribution

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func installedFixture(t *testing.T) (string, string) {
	t.Helper()
	root, license := fixture(t)
	archive := filepath.Join(t.TempDir(), "reference.zip")
	if err := Build(root, license, archive, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	return extractTestPackage(t, archive), archive
}

func extractTestPackage(t *testing.T, archive string) string {
	t.Helper()
	installed := t.TempDir()
	z, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, entry := range z.File {
		path := filepath.Join(installed, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return installed
}

func TestInstalledVerificationPreservesExtrasAndRejectsChanges(t *testing.T) {
	for _, scenario := range []string{"valid", "changed", "missing", "manifest", "pending"} {
		t.Run(scenario, func(t *testing.T) {
			dir, archive := installedFixture(t)
			extra := filepath.Join(dir, "user.txt")
			if err := os.WriteFile(extra, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "changed":
				if err := os.WriteFile(filepath.Join(dir, "WinOne.exe"), []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(filepath.Join(dir, "WinOne.exe")); err != nil {
					t.Fatal(err)
				}
			case "manifest":
				if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schemaVersion":2}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "pending":
				if err := os.WriteFile(filepath.Join(dir, ".winone-pending"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := VerifyInstalled(dir, archive)
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: %v", scenario, err)
			}
			if data, err := os.ReadFile(extra); err != nil || string(data) != "keep" {
				t.Fatal("extra changed", err)
			}
		})
	}
}

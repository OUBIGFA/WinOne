package distribution

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPackagePaths(t *testing.T) {
	for _, name := range []string{"capture/CON.dll", "capture/aux.resources.dll", "capture/CONIN$.dll", "capture/AUX .dll", "capture/a.dll ", "capture/\x00.dll", "capture/../WinOne.exe", "C:/evil.dll"} {
		if safeName(name) {
			t.Fatalf("unsafe Windows path accepted: %q", name)
		}
	}
	if !safeName("capture/zh-CN/ShareX.HelpersLib.resources.dll") {
		t.Fatal("valid resource path rejected")
	}
}

func TestVerifyRejectsChangedPayloadAndTrailingManifest(t *testing.T) {
	root, license := fixture(t)
	valid := filepath.Join(t.TempDir(), "valid.zip")
	if err := Build(root, license, valid, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"WinOne.exe", "manifest.json"} {
		archive, err := zip.OpenReader(valid)
		if err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(t.TempDir(), "changed.zip")
		file, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(file)
		for _, entry := range archive.File {
			reader, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if entry.Name == target {
				if target == "manifest.json" {
					data = append(data, []byte(`{}`)...)
				} else {
					data[0] ^= 1
				}
			}
			destination, err := writer.Create(entry.Name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = destination.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		writer.Close()
		file.Close()
		archive.Close()
		if _, err = Verify(output); err == nil {
			t.Fatalf("modified %s accepted", target)
		}
	}
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range requiredFiles {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	license := filepath.Join(t.TempDir(), "LICENSE")
	if err := os.WriteFile(license, []byte("test license"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecordBuild(root, license, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	return root, license
}

func TestPackageRejectsRelabelledOrModifiedBuild(t *testing.T) {
	root, license := fixture(t)
	if err := Build(root, license, filepath.Join(t.TempDir(), "wrong-version.zip"), "0.2.0"); err == nil {
		t.Fatal("old build relabelled as a new version")
	}
	if err := os.WriteFile(filepath.Join(root, "WinOne.Host.exe"), []byte("changed since build"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Build(root, license, filepath.Join(t.TempDir(), "changed.zip"), "0.1.0-alpha.1"); err == nil {
		t.Fatal("changed build was packaged")
	}
}
func TestPackageRoundTripExcludesSymbolsAndIsRepeatable(t *testing.T) {
	root, license := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "capture", "debug.pdb"), []byte("machine path"), 0600); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(t.TempDir(), "first.zip"), filepath.Join(t.TempDir(), "second.zip")
	if err := Build(root, license, first, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	manifest, err := Verify(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Files {
		if filepath.Ext(entry.Path) == ".pdb" {
			t.Fatal("debug symbols packaged")
		}
	}
	if manifest.Version != "0.1.0-alpha.1" {
		t.Fatal("version lost")
	}
	if err = Build(root, license, second, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	one, _ := os.ReadFile(first)
	two, _ := os.ReadFile(second)
	if string(one) != string(two) {
		t.Fatal("identical inputs produced different archives")
	}
	if err = Build(root, license, first, "0.1.0-alpha.1"); err == nil {
		t.Fatal("overwrote existing package")
	}
}
func TestPackageRejectsLocalDataAndIncompleteBundle(t *testing.T) {
	root, license := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "capture", "settings.json"), []byte("private data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Build(root, license, filepath.Join(t.TempDir(), "bad.zip"), "0.1.0-alpha.1"); err == nil {
		t.Fatal("local data accepted")
	}
	if err := Build(t.TempDir(), license, filepath.Join(t.TempDir(), "empty.zip"), "0.1.0-alpha.1"); err == nil {
		t.Fatal("empty bundle accepted")
	}
}

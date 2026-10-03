package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerArtifactsBindPackageAndBothExecutables(t *testing.T) {
	source, license := fixture(t)
	archive := filepath.Join(t.TempDir(), "package.zip")
	if err := Build(source, license, archive, "0.1.0-alpha.1"); err != nil {
		t.Fatal(err)
	}
	manifest, err := Verify(archive)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(manifest, "", "  ")
	digest := sha256.Sum256(raw)
	testBuild := true
	record := InstallerArtifacts{SchemaVersion: 1, Version: manifest.Version, Platform: manifest.Platform, PackageManifestSHA256: hex.EncodeToString(digest[:]), TestBuild: &testBuild}
	directory := t.TempDir()
	makeFile := func(name, content string) InstallerArtifactFile {
		data := []byte(content)
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return InstallerArtifactFile{FileName: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	}
	record.Installer = makeFile("WinOne-0.1.0-alpha.1-windows-x64-test-setup.exe", "installer")
	record.Uninstaller = makeFile("WinOne-0.1.0-alpha.1-windows-x64-test-uninstall.exe", "uninstaller")
	path := filepath.Join(directory, "artifacts.json")
	write := func() {
		data, _ := json.Marshal(record)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := VerifyInstallerArtifacts(path, archive, true); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyInstallerArtifacts(path, archive, false); err == nil {
		t.Fatal("test artifacts accepted for production")
	}
	validRecord := record
	for _, mutate := range []func(*InstallerArtifacts){
		func(r *InstallerArtifacts) { r.TestBuild = nil },
		func(r *InstallerArtifacts) { r.Version = "9.0.0" },
		func(r *InstallerArtifacts) { r.Uninstaller.FileName = "../outside.exe" },
		func(r *InstallerArtifacts) { r.Uninstaller.Size = 17 << 20 },
	} {
		record = validRecord
		mutate(&record)
		write()
		if _, err := VerifyInstallerArtifacts(path, archive, true); err == nil {
			t.Fatal("invalid artifact metadata accepted")
		}
	}
	record = validRecord
	record.PackageManifestSHA256 = "bad"
	write()
	if _, err := VerifyInstallerArtifacts(path, archive, true); err == nil {
		t.Fatal("wrong package accepted")
	}
	record.PackageManifestSHA256 = hex.EncodeToString(digest[:])
	write()
	if err := os.WriteFile(filepath.Join(directory, record.Uninstaller.FileName), []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyInstallerArtifacts(path, archive, true); err == nil {
		t.Fatal("changed uninstaller accepted")
	}
}

func TestNativeInstallerArtifactRecord(t *testing.T) {
	record, archive := os.Getenv("WINONE_INSTALLER_ARTIFACT_RECORD"), os.Getenv("WINONE_INSTALLER_ARTIFACT_PACKAGE")
	if record == "" || archive == "" {
		t.Skip("set generated artifact record and package paths")
	}
	if _, err := VerifyInstallerArtifacts(record, archive, true); err != nil {
		t.Fatal(err)
	}
}

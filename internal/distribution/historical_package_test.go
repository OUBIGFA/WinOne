package distribution

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Build an old, self-describing release without asking the current packaging
// inventory to manufacture a dependency it intentionally no longer ships.
func historicalPackageFixture(t *testing.T, legacy string) string {
	t.Helper()
	manifest := newManifest("0.1.0-alpha.1")
	payload := map[string][]byte{}
	for _, name := range append(append([]string{}, requiredFiles...), "LICENSE.txt", "README.txt", legacy) {
		payload[name] = []byte(name)
		digest := sha256.Sum256(payload[name])
		manifest.Files = append(manifest.Files, File{Path:name, Size:int64(len(payload[name])), SHA256:hex.EncodeToString(digest[:])})
	}
	raw, err := json.Marshal(manifest)
	if err != nil { t.Fatal(err) }
	payload["manifest.json"] = raw
	path := filepath.Join(t.TempDir(), "historical.zip")
	file, err := os.Create(path)
	if err != nil { t.Fatal(err) }
	writer := zip.NewWriter(file)
	for name, data := range payload {
		entry, err := writer.Create(name)
		if err != nil { t.Fatal(err) }
		if _, err = entry.Write(data); err != nil { t.Fatal(err) }
	}
	if err = writer.Close(); err != nil { t.Fatal(err) }
	if err = file.Close(); err != nil { t.Fatal(err) }
	return path
}

func TestUpgradeAcceptsHistoricalDependenciesButNewPackagesStayStrict(t *testing.T) {
	for _, name := range []string{"capture/Ocr/python/Lib/site-packages/retired-1.0.dist-info/METADATA", "shared/ai/plugins/retired-tool/index.mjs", "shared/chat/notices/retired-license.txt"} {
		t.Run(name, func(t *testing.T) {
			previous := historicalPackageFixture(t, name)
			if _, err := Verify(previous); err == nil { t.Fatal("current package gate accepted a retired/incomplete runtime") }
			root, license := fixture(t)
			next := filepath.Join(t.TempDir(), "current.zip")
			if err := Build(root, license, next, "0.1.0-alpha.1"); err != nil { t.Fatal(err) }
			plan, err := PlanInstallation(previous, next)
			if err != nil { t.Fatal("old dependency prevented reinstall:", err) }
			found := false
			for _, change := range plan.Files { if change.Path == name { found = change.Action == "remove" } }
			if !found { t.Fatal("old dependency was not included in the exact removal plan") }
			if _, err = CreateUpgradeJournal(filepath.Join(t.TempDir(), "journal.json"), plan); err != nil { t.Fatal(err) }
			installed := extractTestPackage(t, previous)
			backup := filepath.Join(t.TempDir(), "backup")
			if _, err = BackupInstalled(installed, previous, backup); err != nil { t.Fatal(err) }
			if _, err = VerifyBackup(backup, previous); err != nil { t.Fatal(err) }
			if err = os.WriteFile(filepath.Join(installed, filepath.FromSlash(name)), []byte("changed"), 0600); err != nil { t.Fatal(err) }
			if _, err = VerifyInstalled(installed, previous); err == nil { t.Fatal("history compatibility ignored a changed file") }
		})
	}
}

func TestHistoricalPackageRejectsUserDataAndUnsafePaths(t *testing.T) {
	for _, name := range []string{"notes.txt", "capture/settings.json", "../outside.txt", "capture/Ocr/../../outside.txt", "capture/Ocr/NUL.txt", "shared/chat/file:stream"} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyHistoricalPackage(historicalPackageFixture(t, name)); err == nil { t.Fatal("unsafe or local-data historical entry accepted") }
		})
	}
}

//go:build windows

package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidationRetiresOnlyKnownDiagnosticsAfterImport(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "modules", "capture", "worker.log")
	writeFile(t, old, "capture failure\n")
	writeFile(t, filepath.Join(dir, "modules", "always-on-top", "native", "native_2026-10-07.log"), "native failure\n")
	database := filepath.Join(dir, "webview-components", "Local Storage", "000003.log")
	writeFile(t, database, "database data")
	unknown := filepath.Join(dir, "modules", "capture", "user.log")
	writeFile(t, unknown, "keep")
	retired := 0
	err := consolidateLegacy(dir, func(path string, check func() error) error {
		if err := check(); err != nil {
			return err
		}
		content := readFile(t, filepath.Join(dir, "logs", "host.log"))
		if !strings.Contains(content, "failure") {
			t.Fatal("original retired before import")
		}
		retired++
		// Unit tests move their owned files to a retained fixture directory.
		return os.Rename(path, path+".recycled-fixture")
	})
	if err != nil || retired != 2 {
		t.Fatal(retired, err)
	}
	if readFile(t, database) != "database data" || readFile(t, unknown) != "keep" {
		t.Fatal("unrelated runtime files changed")
	}
}

func TestConsolidationPreservesOriginalWhenImportFails(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "modules", "wintray", "wintray.log")
	writeFile(t, old, "keep failure")
	if err := os.MkdirAll(filepath.Join(dir, "logs", "host.log"), 0700); err != nil {
		t.Fatal(err)
	}
	err := consolidateLegacy(dir, func(string, func() error) error {
		t.Fatal("retired after failed import")
		return errors.New("unreachable")
	})
	if err == nil || readFile(t, old) != "keep failure" {
		t.Fatal("migration concealed a failed import", err)
	}
}

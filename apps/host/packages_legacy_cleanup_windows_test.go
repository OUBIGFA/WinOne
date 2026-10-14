//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/host"
	"winone/internal/modulepackage"
)

func TestLegacyStageAppearsInCleanupButUnverifiedFilesStay(t *testing.T) {
	p, store, file := packagePreviewFixture(t)
	preview := readyPackagePreview(t, p, store, file)
	directory := filepath.Join(p.data, "packages")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	publisher, err := p.trust("example")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := modulepackage.StageArchive(context.Background(), file, modulepackage.StageOptions{Directory: directory, Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: preview.ManifestSHA256})
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(stage.Directory(), "keep.txt")
	if err = os.WriteFile(keep, []byte("not owned by the package"), 0600); err != nil {
		t.Fatal(err)
	}
	before := store.CommittedDocument().Revision
	if status := p.packageCleanupStatus(); !status.Pending {
		t.Fatal("legacy stage has no cleanup entry", status)
	}
	reply := p.cleanupPackages(host.Request{Method: "cleanupPackages"}, store)
	if reply.Error == nil || !strings.Contains(reply.Error.Message, filepath.Base(stage.Directory())) {
		t.Fatal("unsafe legacy content was hidden or reported cleaned", reply)
	}
	if raw, err := os.ReadFile(keep); err != nil || string(raw) != "not owned by the package" {
		t.Fatal("legacy cleanup changed unknown data", err)
	}
	if store.CommittedDocument().Revision != before || p.Running() {
		t.Fatal("inspection changed settings or executed code")
	}
}

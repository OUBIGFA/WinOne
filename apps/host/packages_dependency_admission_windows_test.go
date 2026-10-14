//go:build windows

package main

import (
	"aead.dev/minisign"
	"crypto/rand"
	"strings"
	"testing"
	"winone/internal/modulepackage"
)

func TestPreviewDoesNotIgnoreUnresolvedDependencyDeclarations(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file, publisher := hostPackageArchiveFixtureDefined(t, []byte("inert worker"), public, private, hostPackageArchiveOptions{Dependencies: []modulepackage.Dependency{{Kind: "runtime", ID: "example.shared", Version: "1.0.0", ManifestSHA256: strings.Repeat("a", 64)}}})
	p, store, _ := packagePreviewFixtureFromArchive(t, file, publisher)
	preview := checkedPackagePreview(t, p, store, file)
	if preview.CanInstall || preview.State != "failed" || preview.Error == "" {
		t.Fatal("unresolved dependency became an ordinary installable module", preview)
	}
	if p.Running() {
		t.Fatal("preview launched module code")
	}
}

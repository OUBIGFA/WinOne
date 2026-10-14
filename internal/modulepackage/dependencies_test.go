package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/catalog"
)

func runtimeArchiveFixture(t *testing.T) (string, Publisher) {
	t.Helper()
	manifest, publisher, key := signedTestManifest(t)
	manifest.Format = "winone.runtime.v1"
	manifest.Metadata = runtimeMetadataFixture()
	return writeRuntimeArchive(t, manifest, publisher, key), publisher
}

func writeRuntimeArchive(t *testing.T, manifest Manifest, publisher Publisher, key minisign.PrivateKey) string {
	t.Helper()
	raw, _ := json.Marshal(manifest)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range map[string][]byte{"module.json": raw, "module.json.minisig": minisign.Sign(key, raw), "bin/worker.exe": []byte("inert worker"), "LICENSE.txt": []byte("test license")} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "runtime.zip")
	if err := os.WriteFile(file, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRuntimeArchiveAndStageKeepIdentity(t *testing.T) {
	file, publisher := runtimeArchiveFixture(t)
	preview, err := PreviewArchive(context.Background(), file, func(namespace string) (Publisher, error) {
		if namespace != "example" {
			return Publisher{}, fmt.Errorf("wrong runtime namespace %q", namespace)
		}
		return publisher, nil
	})
	if err != nil || preview.Settings() != nil || preview.Manifest().Metadata().PackageID() != "example.shared" {
		t.Fatal("runtime preview changed identity", err)
	}
	options := StageOptions{Directory: t.TempDir(), Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: preview.Manifest().Digest()}
	stage, err := StageArchive(context.Background(), file, options)
	if err != nil {
		t.Fatal(err)
	}
	if stage.Manifest().Metadata().PackageID() != "example.shared" {
		t.Fatal("runtime stage has no identity")
	}
	if _, err = VerifyStage(context.Background(), filepath.Base(stage.Directory()), options); err != nil {
		t.Fatal(err)
	}
}

func runtimeMetadataFixture() Metadata {
	m := testMetadata()
	m.SchemaVersion = 4
	m.Kind = "runtime"
	m.Module = catalog.Manifest{}
	m.Settings = nil
	m.Runtime = &RuntimeManifest{ID: "example.shared", Name: catalog.LocalizedName{Chinese: "共享资源", English: "Shared resources"}, Exports: []RuntimeExport{{Name: "root", Path: ".", Kind: "directory"}, {Name: "library", Path: "bin/worker.exe", Kind: "file"}}}
	return m
}

func TestRuntimeMetadataSeparatesResourcesFromModules(t *testing.T) {
	m := runtimeMetadataFixture()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.PackageID() != "example.shared" || m.PackageKind() != "runtime" || m.Module.Worker != "" {
		t.Fatal("runtime gained module execution fields")
	}
	for _, change := range []func(*Metadata){
		func(m *Metadata) { m.Module = testMetadata().Module },
		func(m *Metadata) { m.Settings = &SettingsFiles{Schema: "schema.json", Defaults: "defaults.json"} },
		func(m *Metadata) { m.Runtime.Exports = nil },
		func(m *Metadata) { m.Runtime.Exports[0].Path = "../outside" },
		func(m *Metadata) { m.Runtime.Exports[1].Name = m.Runtime.Exports[0].Name },
		func(m *Metadata) { m.Runtime.Exports[0].Kind = "command" },
		func(m *Metadata) { m.SchemaVersion = 3 },
	} {
		value := runtimeMetadataFixture()
		change(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid runtime accepted: %+v", value)
		}
	}
}

func TestRuntimeManifestAuthenticatesExportsAndCopiesMetadata(t *testing.T) {
	manifest, publisher, key := signedTestManifest(t)
	manifest.Format = "winone.runtime.v1"
	manifest.Metadata = runtimeMetadataFixture()
	raw, _ := json.Marshal(manifest)
	verified, err := VerifyManifest(raw, minisign.Sign(key, raw), publisher)
	if err != nil {
		t.Fatal(err)
	}
	copy := verified.Metadata()
	copy.Runtime.Exports[0].Path = "outside"
	if verified.Metadata().Runtime.Exports[0].Path != "." {
		t.Fatal("caller changed runtime metadata")
	}
	manifest.Metadata.Runtime.Exports[1].Path = "missing.exe"
	raw, _ = json.Marshal(manifest)
	if _, err = VerifyManifest(raw, minisign.Sign(key, raw), publisher); err == nil {
		t.Fatal("unlisted runtime export accepted")
	}
	manifest.Metadata = runtimeMetadataFixture()
	manifest.Format = "winone.module.v1"
	raw, _ = json.Marshal(manifest)
	if _, err = VerifyManifest(raw, minisign.Sign(key, raw), publisher); err == nil {
		t.Fatal("runtime masqueraded as a module archive")
	}
}

func TestLockedDependencyDeclarationsRejectAmbiguity(t *testing.T) {
	valid := Dependency{Kind: "runtime", ID: "example.base", Version: "1.2.0", ManifestSHA256: strings.Repeat("a", 64)}
	m := runtimeMetadataFixture()
	m.Dependencies = []Dependency{valid}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Metadata){
		func(m *Metadata) { m.Dependencies[0].Kind = "process" },
		func(m *Metadata) { m.Dependencies[0].Version = "latest" },
		func(m *Metadata) { m.Dependencies[0].ManifestSHA256 = "wrong" },
		func(m *Metadata) { m.Dependencies[0].ID = m.PackageID() },
		func(m *Metadata) { m.Dependencies = append(m.Dependencies, m.Dependencies[0]) },
	} {
		value := runtimeMetadataFixture()
		value.Dependencies = []Dependency{valid}
		change(&value)
		if err := value.Validate(); err == nil {
			t.Fatal("ambiguous dependency was accepted")
		}
	}
	old := testMetadata()
	old.Dependencies = []Dependency{valid}
	if err := old.Validate(); err == nil {
		t.Fatal("legacy metadata silently accepted dependency declarations")
	}
}

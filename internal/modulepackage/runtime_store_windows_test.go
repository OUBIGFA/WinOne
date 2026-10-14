//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeStorePersistsExactVersionsWithoutModuleState(t *testing.T) {
	file, publisher := runtimeArchiveFixture(t)
	verified, err := VerifyArchive(context.Background(), file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref := graphReference(verified.Manifest())
	options := RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: ref.ManifestSHA256}
	installed, err := store.Install(context.Background(), file, options)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Install(context.Background(), file, options)
	if err != nil || again != installed {
		t.Fatal("same runtime was copied again", again, installed, err)
	}
	reopened, err := OpenRuntimeStore(store.packagesDirectory)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Lookup(context.Background(), ref, publisher, testEnvironment())
	if err != nil || loaded.Manifest().Digest() != ref.ManifestSHA256 {
		t.Fatal("stored runtime was not verified", err)
	}
	snapshot, err := reopened.Snapshot()
	if err != nil || len(snapshot.Records) != 1 {
		t.Fatal(snapshot, err)
	}
	if _, err = os.Stat(filepath.Join(store.packagesDirectory, "module-state.json")); !os.IsNotExist(err) {
		t.Fatal("runtime installation created module intent", err)
	}
	ledger, err := store.journal.Reclamation()
	if err != nil || len(ledger.Items) != 0 {
		t.Fatal("registered runtime retained preparation cleanup", ledger, err)
	}
	wrong := ref
	wrong.Version = "9.0.0"
	if _, err = reopened.Lookup(context.Background(), wrong, publisher, testEnvironment()); err == nil {
		t.Fatal("runtime version mismatch ignored")
	}
}

func TestRuntimeStoreFailureLeavesPreparationAndNoIndexCommit(t *testing.T) {
	file, publisher := runtimeArchiveFixture(t)
	verified, err := VerifyArchive(context.Background(), file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenRuntimeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(store.directory, runtimeIndexName), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(context.Background(), file, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()})
	if err == nil {
		t.Fatal("invalid runtime inventory was ignored")
	}
	// Failure before trusted index loading must not touch or reset that index.
	if info, err := os.Stat(filepath.Join(store.directory, runtimeIndexName)); err != nil || !info.IsDir() {
		t.Fatal("invalid inventory was replaced", err)
	}
}

func TestRuntimeIndexRejectsDuplicateVersionContent(t *testing.T) {
	value := RuntimeSnapshot{SchemaVersion: 1, Records: map[string]RegisteredPackage{}}
	for index, digest := range []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"} {
		value.Records[digest] = RegisteredPackage{ModuleID: "example.shared", Version: "1.0.0", ManifestSHA256: digest, StageID: []string{"stage-00000000000000000000000000000001", "stage-00000000000000000000000000000002"}[index]}
	}
	if err := validateRuntimeSnapshot(value); err == nil {
		t.Fatal("different content uses the same runtime version")
	}
	second := value.Records["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]
	second.Version = "2.0.0"
	value.Records[second.ManifestSHA256] = second
	if err := validateRuntimeSnapshot(value); err != nil {
		t.Fatal("side-by-side versions rejected", err)
	}
	if _, err := json.Marshal(value); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStoreKeepsBothSignedVersionsAndRejectsReusedVersion(t *testing.T) {
	manifest, publisher, key := signedTestManifest(t)
	manifest.Format = "winone.runtime.v1"
	manifest.Metadata = runtimeMetadataFixture()
	store, err := OpenRuntimeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var installed []RegisteredPackage
	for _, version := range []string{"1.2.0", "1.3.0"} {
		manifest.Metadata.Version = version
		file := writeRuntimeArchive(t, manifest, publisher, key)
		verified, err := VerifyArchive(context.Background(), file, publisher)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Install(context.Background(), file, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()})
		if err != nil {
			t.Fatal(err)
		}
		installed = append(installed, record)
	}
	if installed[0].StageID == installed[1].StageID {
		t.Fatal("new runtime replaced the old directory")
	}
	for _, record := range installed {
		if _, err = store.Lookup(context.Background(), Dependency{Kind: "runtime", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256}, publisher, testEnvironment()); err != nil {
			t.Fatal("coexisting runtime lost its content", err)
		}
	}
	manifest.Metadata.Runtime.Exports[1].Name = "changed-export"
	file := writeRuntimeArchive(t, manifest, publisher, key)
	verified, err := VerifyArchive(context.Background(), file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Install(context.Background(), file, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()}); err == nil {
		t.Fatal("same version acquired different content")
	}
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Records) != 2 {
		t.Fatal("rejected runtime changed stored versions", snapshot, err)
	}
}

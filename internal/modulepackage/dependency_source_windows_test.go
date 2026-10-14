//go:build windows

package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/distribution"
	"winone/internal/modulestate"
)

func dependencyModuleArchive(t *testing.T, id string, dependencies []Dependency, base Manifest, publisher Publisher, key minisign.PrivateKey) string {
	t.Helper()
	base.Format = "winone.module.v1"
	base.Metadata = testMetadata()
	base.Metadata.SchemaVersion = 4
	base.Metadata.Kind = "module"
	base.Metadata.Module.ID = id
	base.Metadata.Dependencies = dependencies
	base.Metadata.Settings = &SettingsFiles{Schema: "schema.json", Defaults: "defaults.json"}
	files := map[string][]byte{"bin/worker.exe": []byte("inert worker"), "LICENSE.txt": []byte("test license"), "schema.json": []byte(`{"type":"object"}`), "defaults.json": []byte(`{}`)}
	base.Files = nil
	for name, data := range files {
		sum := sha256.Sum256(data)
		base.Files = append(base.Files, distribution.File{Path: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	files["module.json"] = raw
	files["module.json.minisig"] = minisign.Sign(key, raw)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "module.zip")
	if err = os.WriteFile(file, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestDependencyReferencesSurvivePendingRemovalAndProtectOtherModules(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	modules, err := OpenRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := OpenRuntimeStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	base, publisher, key := signedTestManifest(t)
	runtimeManifest := base
	runtimeManifest.Format = "winone.runtime.v1"
	runtimeManifest.Metadata = runtimeMetadataFixture()
	file := writeRuntimeArchive(t, runtimeManifest, publisher, key)
	verified, err := VerifyArchive(ctx, file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	reference := graphReference(verified.Manifest())
	if _, err = runtimes.Install(ctx, file, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: reference.ManifestSHA256}); err != nil {
		t.Fatal(err)
	}
	source, err := NewDependencySource(modules, runtimes, testEnvironment(), func(string) (Publisher, error) { return publisher, nil })
	if err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"example.first", "example.second"} {
		archive := dependencyModuleArchive(t, id, []Dependency{reference}, base, publisher, key)
		checked, err := VerifyArchive(ctx, archive, publisher)
		if err != nil {
			t.Fatal(err)
		}
		options := StageOptions{Directory: directory, Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: checked.Manifest().Digest(), Dependencies: source}
		stage, err := StageArchive(ctx, archive, options)
		if err != nil {
			t.Fatal(err)
		}
		current, err := modules.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = modules.PrepareModule(ctx, filepath.Base(stage.Directory()), options, current.Revision, state); err != nil {
			t.Fatal(err)
		}
	}
	assertReferences := func(count int) {
		t.Helper()
		refs, err := source.References(ctx, reference)
		if err != nil || len(refs) != count {
			t.Fatal("wrong dependency reference set", refs, err)
		}
	}
	assertReferences(2)
	unused := reference
	unused.ID = "example.unused"
	index, err := source.ReferencesFor(ctx, []Dependency{reference, unused})
	if err != nil || len(index[reference]) != 2 || len(index[unused]) != 0 {
		t.Fatal("batched reference results were mixed", index, err)
	}
	if removed, err := runtimes.Remove(ctx, reference, source, publisher); err == nil || removed {
		t.Fatal("a shared runtime used by two modules was unregistered", removed, err)
	}
	current, err := modules.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	change := ChangeRequest{ID: "remove-first", RegistryRevision: current.Revision, StateRevision: state.Snapshot().Revision, Current: current.Records["example.first"], Dependencies: source}
	if _, err = modules.BeginChange(ctx, change, state); err != nil {
		t.Fatal(err)
	}
	assertReferences(2)
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.first", false); err != nil {
		t.Fatal(err)
	}
	if _, err = state.FinishStop("example.first"); err != nil {
		t.Fatal(err)
	}
	if _, err = modules.ApplyChange(ctx, change.ID, state, nil); err != nil {
		t.Fatal(err)
	}
	assertReferences(2)
	if _, err = modules.FinishChange(ctx, change.ID, state, nil); err != nil {
		t.Fatal(err)
	}
	assertReferences(1)
	if removed, err := runtimes.Remove(ctx, reference, source, publisher); err == nil || removed {
		t.Fatal("remaining module lost its runtime", removed, err)
	}
	if _, err = runtimes.Lookup(ctx, reference, publisher, testEnvironment()); err != nil {
		t.Fatal("removing one module changed the shared runtime", err)
	}
	current, err = modules.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, current.Records["example.second"].StageID, "module.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("damaged metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = source.References(ctx, reference); err == nil {
		t.Fatal("damaged reference metadata was treated as no users")
	}
	if _, err = runtimes.Remove(ctx, reference, source, publisher); err == nil {
		t.Fatal("runtime removed while reference audit was incomplete")
	}
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	change = ChangeRequest{ID: "remove-second", RegistryRevision: current.Revision, StateRevision: state.Snapshot().Revision, Current: current.Records["example.second"], Dependencies: source}
	if _, err = modules.BeginChange(ctx, change, state); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.second", false); err != nil {
		t.Fatal(err)
	}
	if _, err = state.FinishStop("example.second"); err != nil {
		t.Fatal(err)
	}
	if _, err = modules.ApplyChange(ctx, change.ID, state, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = modules.FinishChange(ctx, change.ID, state, nil); err != nil {
		t.Fatal(err)
	}
	assertReferences(0)
	stage, err := runtimes.Lookup(ctx, reference, publisher, testEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	files, err := holdRuntimeFiles(ctx, stage.Directory(), stage.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	if removed, err := runtimes.Remove(ctx, reference, source, publisher); err != nil || !removed {
		t.Fatal("unused runtime could not be unregistered", removed, err)
	}
	if err = runtimes.Reclaim(ctx, reference, source, publisher); err == nil {
		t.Fatal("running file lease did not block runtime recycling")
	}
	if _, err = os.Stat(stage.Directory()); err != nil {
		t.Fatal("held runtime files disappeared", err)
	}
	remaining, err := runtimes.Snapshot()
	if err != nil || len(remaining.Records) != 0 {
		t.Fatal("runtime index did not reflect removal", remaining, err)
	}
	cleanup, err := runtimes.journal.Reclamation()
	if err != nil || len(cleanup.Items) != 1 {
		t.Fatal("held runtime lost its retryable cleanup record", cleanup, err)
	}
}

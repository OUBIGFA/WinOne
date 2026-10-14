//go:build windows

package modulepackage

import (
	"context"
	"path/filepath"
	"testing"
	"winone/internal/modulestate"
)

func TestModuleDependencyBlocksUpdateAndUninstall(t *testing.T) {
	ctx := context.Background()
	registry, err := OpenRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := OpenRuntimeStore(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	base, publisher, key := signedTestManifest(t)
	source, err := NewDependencySource(registry, runtimes, testEnvironment(), func(string) (Publisher, error) { return publisher, nil })
	if err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dependencies []Dependency
	for _, id := range []string{"example.tool", "example.consumer"} {
		file := dependencyModuleArchive(t, id, dependencies, base, publisher, key)
		checked, err := VerifyArchive(ctx, file, publisher)
		if err != nil {
			t.Fatal(err)
		}
		options := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: checked.Manifest().Digest(), Dependencies: source}
		stage, err := StageArchive(ctx, file, options)
		if err != nil {
			t.Fatal(err)
		}
		current, err := registry.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = registry.PrepareModule(ctx, filepath.Base(stage.Directory()), options, current.Revision, state); err != nil {
			t.Fatal(err)
		}
		dependencies = []Dependency{graphReference(checked.Manifest())}
	}
	stage, approval := replacementStage(t, registry)
	for _, replacement := range []*ReplacementStage{nil, {ID: filepath.Base(stage.Directory()), Options: approval}} {
		for _, supplied := range []*DependencySource{nil, source} {
			request := changeRequest(t, registry, state, replacement)
			request.Dependencies = supplied
			if _, err = registry.BeginChange(ctx, request, state); err == nil {
				t.Fatal("a pinned module was changed while a consumer still required it")
			}
			current, err := registry.Snapshot()
			if err != nil || current.Pending != nil {
				t.Fatal("rejected dependency change altered inventory", current, err)
			}
		}
	}
}

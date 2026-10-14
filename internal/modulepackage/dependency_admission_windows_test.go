//go:build windows

package modulepackage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRuntimeCannotEnterModuleRegistration(t *testing.T) {
	file, publisher := runtimeArchiveFixture(t)
	verified, err := VerifyArchive(context.Background(), file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	options := StageOptions{Directory: t.TempDir(), Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()}
	stage, err := StageArchive(context.Background(), file, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.RegisterStage(context.Background(), filepath.Base(stage.Directory()), options, 0); err == nil {
		t.Fatal("runtime was registered as an executable module")
	}
	snapshot, err := registry.Snapshot()
	if err != nil || len(snapshot.Records) != 0 {
		t.Fatal("rejected runtime changed module inventory", snapshot, err)
	}
}

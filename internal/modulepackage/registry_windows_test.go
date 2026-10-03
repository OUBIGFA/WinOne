//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
	"winone/internal/settings"
)

func registeredFixture(t *testing.T) (*Registry, *StagedPackage, StageOptions) {
	t.Helper()
	source, options := stageFixture(t)
	stage, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	return registry, stage, options
}
func TestRegistryRecordsVerifiedStageAndRetriesWithoutDuplication(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	id := filepath.Base(stage.Directory())
	first, err := registry.RegisterStage(context.Background(), id, options, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || len(first.Records) != 1 || first.Records["example.tool"].StageID != id {
		t.Fatal(first)
	}
	repeated, err := registry.RegisterStage(context.Background(), id, options, 0)
	if err != nil || repeated.Revision != 1 {
		t.Fatal("retry duplicated registration", repeated, err)
	}
	reopened, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := reopened.Resolve(context.Background(), "example.tool", options.Publisher, options.Environment)
	if err != nil || !resolved.Ready() {
		t.Fatal(err)
	}
	if resolved.Manifest().Digest() != options.ExpectedManifestSHA256 {
		t.Fatal("registered package changed")
	}
}
func TestRegistryRejectsStaleOrConflictingRegistration(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	if _, err := registry.RegisterStage(context.Background(), filepath.Base(stage.Directory()), options, 0); err != nil {
		t.Fatal(err)
	}
	source, next := stageFixture(t)
	next.Directory = options.Directory
	candidate, err := StageArchive(context.Background(), source, next)
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(candidate.Directory())
	if _, err = registry.RegisterStage(context.Background(), id, next, 0); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("stale write accepted", err)
	}
	if _, err = registry.RegisterStage(context.Background(), id, next, 1); err == nil {
		t.Fatal("existing module silently replaced")
	}
	snapshot, err := registry.Snapshot()
	if err != nil || snapshot.Revision != 1 || snapshot.Records["example.tool"].StageID != filepath.Base(stage.Directory()) {
		t.Fatal(snapshot, err)
	}
}
func TestRegistryRejectsChangedStageAndPreservesInvalidIndex(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	id := filepath.Base(stage.Directory())
	if err := os.WriteFile(filepath.Join(stage.Directory(), "bin", "worker.exe"), []byte("other worker"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.RegisterStage(context.Background(), id, options, 0); err == nil {
		t.Fatal("modified stage was registered")
	}
	snapshot, err := registry.Snapshot()
	if err != nil || len(snapshot.Records) != 0 {
		t.Fatal(snapshot, err)
	}
	index := filepath.Join(options.Directory, registryFileName)
	if err = os.WriteFile(index, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.RegisterStage(context.Background(), id, options, 0); err == nil {
		t.Fatal("invalid index replaced")
	}
	data, err := os.ReadFile(index)
	if err != nil || string(data) != "broken" {
		t.Fatal("invalid index was overwritten", err)
	}
}
func TestRegistryHonorsLeaseAndFailedIndexReplacement(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	id := filepath.Base(stage.Directory())
	lease, err := maintenance.AcquireMaintenance(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, blocked := registry.RegisterStage(context.Background(), id, options, 0)
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if blocked == nil {
		t.Fatal("concurrent maintenance ignored")
	}
	initial := RegistrySnapshot{SchemaVersion: 1, Records: map[string]RegisteredPackage{}}
	data, _ := json.Marshal(initial)
	index := filepath.Join(options.Directory, registryFileName)
	if err = os.WriteFile(index, data, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(index)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := registry.RegisterStage(context.Background(), id, options, 0)
	closeErr := windows.CloseHandle(handle)
	if writeErr == nil || closeErr != nil {
		t.Fatal("locked index replacement did not fail", writeErr, closeErr)
	}
	snapshot, err := registry.Snapshot()
	if err != nil || snapshot.Revision != 0 || len(snapshot.Records) != 0 {
		t.Fatal("failed commit changed index", snapshot, err)
	}
}

func TestRegistryResolutionDoesNotTrustChangedRegisteredFiles(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	if _, err := registry.RegisterStage(context.Background(), filepath.Base(stage.Directory()), options, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage.Directory(), "bin", "worker.exe"), []byte("other worker"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(context.Background(), "example.tool", options.Publisher, options.Environment); err == nil {
		t.Fatal("registered flag bypassed content verification")
	}
	snapshot, err := registry.Snapshot()
	if err != nil || snapshot.Revision != 1 || len(snapshot.Records) != 1 {
		t.Fatal("read verification modified registry", snapshot, err)
	}
}

func TestRegistryNamespaceValidationChecksDescendantsAcrossSiblingNames(t *testing.T) {
	snapshot := RegistrySnapshot{SchemaVersion: 1, Records: map[string]RegisteredPackage{}}
	for index, id := range []string{"example.tool", "example.tool-child", "example.tool.child"} {
		snapshot.Records[id] = RegisteredPackage{ModuleID: id, Version: "1.0.0", StageID: fmt.Sprintf("stage-%032x", index+1), ManifestSHA256: fmt.Sprintf("%064x", index+1)}
	}
	if err := validateRegistry(snapshot); err == nil {
		t.Fatal("sibling sorting concealed a namespace overlap")
	}
	delete(snapshot.Records, "example.tool.child")
	if err := validateRegistry(snapshot); err != nil {
		t.Fatal("unrelated sibling rejected", err)
	}
}

//go:build windows

package modulepackage

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func reclaimFixture(t *testing.T) (*Registry, *StagedPackage, RuntimeOptions, string) {
	t.Helper()
	registry, stage, options := runtimeFixture(t)
	ctx := context.Background()
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.FinishChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil {
		t.Fatal(err)
	}
	for key := range ledger.Items {
		return registry, stage, options, key
	}
	t.Fatal("removal did not queue its stage")
	return nil, nil, RuntimeOptions{}, ""
}

func TestReclaimResumesAfterMoveAndLostFinalWrite(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit recycle-bin integration required")
	}
	for _, lostWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("lost-write-%v", lostWrite), func(t *testing.T) {
			registry, stage, options, key := reclaimFixture(t)
			ctx := context.Background()
			publisher, _ := options.Trust("example")
			quarantine := filepath.Join(registry.directory, quarantineName(key))
			if err := moveReclaimDirectory(stage.Directory(), quarantine, func() error {
				return verifyReclaimDirectory(ctx, stage.Directory(), ReclamationItemFrom(t, registry, key).Package, publisher)
			}); err != nil {
				t.Fatal(err)
			}
			if lostWrite {
				ledger, err := registry.Reclamation()
				if err != nil {
					t.Fatal(err)
				}
				ledger, err = nextReclamation(ledger)
				if err != nil {
					t.Fatal(err)
				}
				item := ledger.Items[key]
				item.Phase = "recycling"
				item.Origin = quarantine
				ledger.Items[key] = item
				if err = registry.writeReclamation(ctx, ledger); err != nil {
					t.Fatal(err)
				}
				path, _ := windows.UTF16PtrFromString(filepath.Join(registry.directory, reclamationFileName))
				lock, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if err != nil {
					t.Fatal(err)
				}
				result, writeErr := registry.Reclaim(ctx, key, publisher)
				if err = windows.CloseHandle(lock); err != nil {
					t.Fatal(err)
				}
				if writeErr == nil || result.State != "recycled" {
					t.Fatal("fixture did not reach a lost completion write", result, writeErr)
				}
				if _, err = os.Stat(quarantine); !os.IsNotExist(err) {
					t.Fatal("quarantine was not recycled", err)
				}
			}
			reopened, err := OpenRegistry(registry.directory)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reopened.Reclaim(ctx, key, publisher)
			if err != nil || result.State != "recycled" {
				t.Fatal("interrupted reclaim did not resume", result, err)
			}
			if data, err := os.ReadFile(filepath.Join(result.Path, "LICENSE.txt")); err != nil || string(data) != "test license" {
				t.Fatal("recovered receipt does not contain package files", err)
			}
		})
	}
}

func ReclamationItemFrom(t *testing.T, registry *Registry, key string) ReclamationItem {
	t.Helper()
	ledger, err := registry.Reclamation()
	if err != nil {
		t.Fatal(err)
	}
	return ledger.Items[key]
}

func TestReclaimPreservesAdditionalFilesAndReinstalledStage(t *testing.T) {
	registry, stage, options, key := reclaimFixture(t)
	ctx := context.Background()
	publisher, _ := options.Trust("example")
	extra := filepath.Join(stage.Directory(), "user-notes.txt")
	if err := os.WriteFile(extra, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Reclaim(ctx, key, publisher); err == nil {
		t.Fatal("extra content was reclaimed")
	}
	if data, err := os.ReadFile(extra); err != nil || string(data) != "keep me" {
		t.Fatal("extra content moved or changed", err)
	}
	registry, stage, options, key = reclaimFixture(t)
	publisher, _ = options.Trust("example")
	current, _ := registry.Snapshot()
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	if _, err := registry.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, current.Revision, options.State); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Reclaim(ctx, key, publisher)
	if err != nil || result.State != "retained-reference" {
		t.Fatal("current package was not retained", result, err)
	}
	if _, err = os.Stat(stage.Directory()); err != nil {
		t.Fatal(err)
	}
}

func TestReclaimMovesOnlyOwnedPackageToRecycleBin(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit recycle-bin integration required")
	}
	registry, stage, options, key := reclaimFixture(t)
	publisher, _ := options.Trust("example")
	config := filepath.Join(options.DataDirectory, "settings.json")
	if err := os.WriteFile(config, []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Reclaim(context.Background(), key, publisher)
	if err != nil || result.State != "recycled" {
		t.Fatal(result, err)
	}
	if _, err = os.Stat(stage.Directory()); !os.IsNotExist(err) {
		t.Fatal("old stage remains", err)
	}
	if data, err := os.ReadFile(filepath.Join(result.Path, "bin", "worker.exe")); err != nil || string(data) != "inert worker" {
		t.Fatal("package was not recoverable", err)
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != `{"keep":true}` {
		t.Fatal("configuration changed", err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || len(ledger.Items) != 0 {
		t.Fatal("reclaim work was not committed", ledger, err)
	}
	if repeated, err := registry.Reclaim(context.Background(), key, publisher); err != nil || repeated != result {
		t.Fatal("reclaim retry changed outcome", repeated, err)
	}
}

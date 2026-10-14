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
)

func TestCompletedRemovalQueuesOwnedStageOnce(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	ctx := context.Background()
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	finished, err := registry.FinishChange(ctx, request.ID, options.State, nil)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Items) != 1 || ledger.ObservedChangeRevision != finished.LastChange.Revision {
		t.Fatal("retired package not queued", ledger)
	}
	for _, item := range ledger.Items {
		if item.Package.StageID != filepath.Base(stage.Directory()) || item.Phase != "queued" {
			t.Fatal(item)
		}
	}
	before := ledger.Revision
	if _, err = registry.SyncReclamation(ctx); err != nil {
		t.Fatal(err)
	}
	ledger, err = registry.Reclamation()
	if err != nil || ledger.Revision != before {
		t.Fatal("queue replay duplicated work", err)
	}
}

func TestFullReclaimQueueCanDrainBeforeNewReceiptSynchronization(t *testing.T) {
	registry, stage, options, key := reclaimFixture(t)
	ctx := context.Background()
	ledger, err := registry.Reclamation()
	if err != nil {
		t.Fatal(err)
	}
	template := ledger.Items[key]
	for index := 0; len(ledger.Items) < maxReclamationItems; index++ {
		item := template
		item.Package.StageID = fmt.Sprintf("stage-%032x", index)
		ledger.Items[reclaimKey(item)] = item
	}
	if err = registry.writeReclamation(ctx, ledger); err != nil {
		t.Fatal(err)
	}
	publisher, _ := options.Trust("example")
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	current, _ := registry.Snapshot()
	if _, err = registry.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, current.Revision, options.State); err != nil {
		t.Fatal(err)
	}
	request := changeRequest(t, registry, options.State, nil)
	request.ID = "remove-again"
	if _, err = registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	completed, err := registry.FinishChange(ctx, request.ID, options.State, nil)
	if !errors.Is(err, ErrReclamationPending) || completed.LastChange == nil || completed.Pending != nil {
		t.Fatal("full queue did not preserve the completed change", err)
	}
	if _, err = registry.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, completed.Revision, options.State); err != nil {
		t.Fatal(err)
	}
	if result, err := registry.Reclaim(ctx, key, publisher); err != nil || result.State != "retained-reference" {
		t.Fatal("full queue could not be drained", result, err)
	}
	resumed, err := registry.SyncReclamation(ctx)
	if err != nil || len(resumed.Items) != maxReclamationItems || resumed.ObservedChangeRevision != completed.LastChange.Revision {
		t.Fatal("new receipt was not recovered after draining", resumed, err)
	}
}

func TestReclamationWriteFailureCannotLoseCompletedChange(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	ctx := context.Background()
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	empty := ReclamationSnapshot{SchemaVersion: 1, Items: map[string]ReclamationItem{}}
	raw, _ := json.Marshal(empty)
	path := filepath.Join(registry.directory, reclamationFileName)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	wide, _ := windows.UTF16PtrFromString(path)
	lock, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	finished, writeErr := registry.FinishChange(ctx, request.ID, options.State, nil)
	if err = windows.CloseHandle(lock); err != nil {
		t.Fatal(err)
	}
	if writeErr == nil || finished.LastChange == nil || finished.Pending != nil {
		t.Fatal("queue failure obscured committed package state", finished, writeErr)
	}
	reopened, err := OpenRegistry(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := reopened.SyncReclamation(ctx)
	if err != nil || len(ledger.Items) != 1 {
		t.Fatal("completed change lost its reclaim work", ledger, err)
	}
}

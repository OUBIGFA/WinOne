//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/settings"
)

func TestPackageChangeRejectsStaleAndForeignPlans(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	ctx := context.Background()
	request := changeRequest(t, registry, options.State, nil)
	stale := request
	stale.StateRevision++
	if _, err := registry.BeginChange(ctx, stale, options.State); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("stale intent revision accepted", err)
	}
	stale = request
	stale.Current.StageID = "stage-00000000000000000000000000000000"
	if _, err := registry.BeginChange(ctx, stale, options.State); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("another installation was accepted", err)
	}
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	other := request
	other.ID = "another-change"
	if _, err := registry.BeginChange(ctx, other, options.State); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("overlapping operation accepted", err)
	}
	publisher, _ := options.Trust("example")
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	snapshot, _ := registry.Snapshot()
	if _, err := registry.RegisterStage(ctx, filepath.Base(stage.Directory()), approval, snapshot.Revision); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("registration bypassed package recovery", err)
	}
	if _, err := registry.ApplyChange(ctx, "wrong-id", options.State, nil); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("wrong operation ID accepted", err)
	}
}

func TestPackageChangeCorruptJournalDoesNotOpenOrRewrite(t *testing.T) {
	for _, mutation := range []string{"phase", "inventory", "revision", "legacy"} {
		t.Run(mutation, func(t *testing.T) {
			registry, _, options := runtimeFixture(t)
			request := changeRequest(t, registry, options.State, nil)
			snapshot, err := registry.BeginChange(context.Background(), request, options.State)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "phase":
				snapshot.Pending.Phase = "finished"
			case "inventory":
				delete(snapshot.Records, "example.tool")
			case "revision":
				snapshot.Pending.RegistryRevision--
			case "legacy":
				snapshot.SchemaVersion = 1
			}
			raw, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(registry.directory, registryFileName)
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenRegistry(registry.directory); err == nil {
				t.Fatal("inconsistent journal accepted")
			}
			if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
				t.Fatal("invalid journal was silently replaced", err)
			}
		})
	}
}

func TestPackageRegistryReadsLegacyWithoutWritingAndMovesToV2OnCommit(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	snapshot, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SchemaVersion = 1
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(registry.directory, registryFileName)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenRegistry(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	read, err := reopened.Snapshot()
	if err != nil || read.SchemaVersion != 2 {
		t.Fatal(read, err)
	}
	if unchanged, err := os.ReadFile(path); err != nil || string(unchanged) != string(raw) {
		t.Fatal("opening legacy inventory rewrote it", err)
	}
	request := changeRequest(t, reopened, options.State, nil)
	if _, err = reopened.BeginChange(context.Background(), request, options.State); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored RegistrySnapshot
	if err = json.Unmarshal(written, &stored); err != nil || stored.SchemaVersion != 2 || stored.Pending == nil {
		t.Fatal("change did not persist v2", err)
	}
}

func TestPackageChangeDoesNotCancelChangedIntentOrAcceptForgedReceipt(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	ctx := context.Background()
	if _, err := options.State.SetIntent(options.State.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	stopChangeState(t, options.State)
	if _, err := registry.CancelChange(ctx, request.ID, options.State); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("cancellation discarded changed intent", err)
	}
	publisher, _ := options.Trust("example")
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	if _, err := registry.RestoreChange(ctx, request.ID, options.State, approval); err != nil {
		t.Fatal(err)
	}
	if _, err := options.State.SetIntent(options.State.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	finished, err := registry.FinishChange(ctx, request.ID, options.State, &approval)
	if err != nil {
		t.Fatal(err)
	}
	finished.LastChange.Revision = finished.LastChange.Change.RegistryRevision + 1
	raw, err := json.Marshal(finished)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(registry.directory, registryFileName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenRegistry(registry.directory); err == nil {
		t.Fatal("forged completion revision was accepted")
	}
}

//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparationTracksFailedAndReadyStagesAcrossReopen(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "bad-payload"}[broken], func(t *testing.T) {
			source, options := stageFixture(t)
			if broken {
				data, publisher := archiveFixture(t, func(files map[string][]byte) { files["bin/worker.exe"] = []byte("damaged") }, false)
				options.Publisher = publisher
				if err := os.WriteFile(source, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := OpenRegistry(options.Directory)
			if err != nil {
				t.Fatal(err)
			}
			stage, err := registry.StageArchive(context.Background(), source, options, "install-one")
			if (err != nil) != broken || stage == nil {
				t.Fatal(stage, err)
			}
			reopened, err := OpenRegistry(options.Directory)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := reopened.SyncReclamation(context.Background())
			if err != nil || len(ledger.Items) != 1 {
				t.Fatal("preparation disappeared", ledger, err)
			}
			for key, item := range ledger.Items {
				if item.Kind != "preparation" || item.ChangeID != "install-one" || item.Package.StageID != filepath.Base(stage.Directory()) {
					t.Fatal(item)
				}
				if broken {
					if _, err = reopened.Reclaim(context.Background(), key, options.Publisher); err == nil {
						t.Fatal("incomplete files were silently recycled")
					}
					if _, err = os.Stat(stage.Directory()); err != nil {
						t.Fatal("incomplete stage was removed", err)
					}
				}
			}
		})
	}
}

func TestPreparationRecorderFailurePrecedesPayloadWrites(t *testing.T) {
	source, options := stageFixture(t)
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the journal path makes atomic persistence fail without
	// touching any user's file or depending on machine ACL configuration.
	if err = os.Mkdir(filepath.Join(options.Directory, reclamationFileName), 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := registry.StageArchive(context.Background(), source, options, "install-one")
	if err == nil || stage == nil || stage.Ready() {
		t.Fatal("payload started without recovery bookkeeping", err)
	}
	entries, err := os.ReadDir(stage.Directory())
	if err != nil || len(entries) != 0 {
		t.Fatal("payload was written before the recovery record", entries, err)
	}
}

func TestPreparationHandsOffToRegistrationWithoutReclaimingFiles(t *testing.T) {
	source, options := stageFixture(t)
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := registry.StageArchive(context.Background(), source, options, "install-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.RegisterStage(context.Background(), filepath.Base(stage.Directory()), options, 0); err != nil {
		t.Fatal(err)
	}
	if err = registry.FinishPreparation(context.Background(), filepath.Base(stage.Directory())); err != nil {
		t.Fatal(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || len(ledger.Items) != 0 {
		t.Fatal("registered stage still shown as abandoned", ledger, err)
	}
	if _, err = VerifyStage(context.Background(), filepath.Base(stage.Directory()), options); err != nil {
		t.Fatal("registration cleanup changed payload", err)
	}
}

func TestPreparationMigrationBindingCannotChangeOrCrossOperations(t *testing.T) {
	source, options := stageFixture(t)
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := registry.StageArchive(context.Background(), source, options, "update-one")
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(stage.Directory())
	digest := strings.Repeat("a", 64)
	if err = registry.BindPreparationMigration(context.Background(), id, "update-one", digest); err != nil {
		t.Fatal(err)
	}
	before, _ := registry.Reclamation()
	for _, values := range [][2]string{{"wrong-operation", digest}, {"update-one", strings.Repeat("b", 64)}} {
		if err = registry.BindPreparationMigration(context.Background(), id, values[0], values[1]); err == nil {
			t.Fatal("migration ownership changed")
		}
	}
	if err = registry.BindPreparationMigration(context.Background(), id, "update-one", digest); err != nil {
		t.Fatal(err)
	}
	after, err := registry.Reclamation()
	if err != nil || after.Revision != before.Revision {
		t.Fatal("idempotent migration binding changed the ledger", after, err)
	}
	if err = registry.FinishPreparation(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	after, err = registry.Reclamation()
	if err != nil || len(after.Items) != 1 {
		t.Fatal("unregistered stage lost its backup reference", after, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = registry.FinishPreparation(ctx, id); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPreparationTransfersMigrationOwnershipToPendingChange(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	candidate, approval := replacementStage(t, registry, "change-1")
	stageID := filepath.Base(candidate.Directory())
	digest := strings.Repeat("a", 64)
	if err := registry.BindPreparationMigration(context.Background(), stageID, "change-1", digest); err != nil {
		t.Fatal(err)
	}
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: stageID, Options: approval})
	request.MigrationSHA256 = digest
	pending, err := registry.BeginChange(context.Background(), request, options.State)
	if err != nil || pending.Pending == nil || pending.Pending.MigrationSHA256 != digest {
		t.Fatal("pending change lost backup ownership", pending, err)
	}
	ledger, err := registry.SyncReclamation(context.Background())
	if err != nil || len(ledger.Items) != 0 {
		t.Fatal("cleanup could target a pending migration", ledger, err)
	}
	stopChangeState(t, options.State)
	if _, err = registry.ApplyChange(context.Background(), request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.FinishChange(context.Background(), request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	ledger, err = registry.Reclamation()
	if err != nil || ledger.SchemaVersion != 3 || len(ledger.Items) != 1 {
		t.Fatal("completed migration did not preserve ledger format", ledger, err)
	}
	for _, item := range ledger.Items {
		if item.Kind != "" || item.MigrationSHA256 != digest || item.Package.StageID == stageID {
			t.Fatal("wrong package or backup queued", item)
		}
	}
}

//go:build windows

package modulepackage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageChangeBindsMigrationBackupAndKeepsItForCleanup(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	candidate, approval := replacementStage(t, registry)
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(candidate.Directory()), Options: approval})
	request.MigrationSHA256 = strings.Repeat("a", 64)
	prepared, err := registry.BeginChange(context.Background(), request, options.State)
	if err != nil || prepared.SchemaVersion != 3 || prepared.Pending.MigrationSHA256 != request.MigrationSHA256 {
		t.Fatal("migration backup not bound to package operation", err)
	}
	changed := request
	changed.MigrationSHA256 = strings.Repeat("b", 64)
	if _, err = registry.BeginChange(context.Background(), changed, options.State); err == nil {
		t.Fatal("a replay changed the migration backup")
	}
	stopChangeState(t, options.State)
	if _, err = registry.ApplyChange(context.Background(), request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	finished, err := registry.FinishChange(context.Background(), request.ID, options.State, &approval)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || ledger.SchemaVersion != 2 || len(ledger.Items) != 1 {
		t.Fatal("migration cleanup record missing", err)
	}
	for _, item := range ledger.Items {
		if item.MigrationSHA256 != request.MigrationSHA256 {
			t.Fatal("cleanup lost migration identity")
		}
	}
	finished.SchemaVersion = 2
	if err = validateRegistry(finished); err == nil {
		t.Fatal("legacy registry accepted a migration-bearing receipt")
	}
}

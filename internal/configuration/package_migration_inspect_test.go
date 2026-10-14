package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageMigrationInspectionIsBoundedReadOnlyAndFindsDamagedRecords(t *testing.T) {
	store, snapshot := migrationFixture(t)
	profile := filepath.Dir(store.store.path)
	if ids, err := PackageMigrationOperations(context.Background(), profile); err != nil || len(ids) != 0 {
		t.Fatal(ids, err)
	}
	digest, err := store.SavePackageMigration(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := store.packageMigrationPath(snapshot.OperationID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	value, inspected, err := store.InspectPackageMigration(snapshot.OperationID)
	if err != nil || inspected != digest || value.OperationID != snapshot.OperationID {
		t.Fatal(value, inspected, err)
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(path), "keep.json"), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	ids, err := PackageMigrationOperations(context.Background(), profile)
	if err != nil || len(ids) != 1 || ids[0] != snapshot.OperationID {
		t.Fatal(ids, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("inspection rewrote backup", err)
	}
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.InspectPackageMigration(snapshot.OperationID); err == nil {
		t.Fatal("damaged record accepted")
	}
	if ids, err = PackageMigrationOperations(context.Background(), profile); err != nil || len(ids) != 1 {
		t.Fatal("damaged record disappeared from inspection", ids, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = PackageMigrationOperations(ctx, profile); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

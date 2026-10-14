package configuration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func migrationFixture(t *testing.T) (*HostStore, PackageMigrationSnapshot) {
	t.Helper()
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RegisterPackageModule(packageAdapter(), store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	document := store.CommittedDocument()
	snapshot := PackageMigrationSnapshot{SchemaVersion: 1, OperationID: "update-fixture", ModuleID: "example.tool", Revision: document.Revision, BeforeSchema: 1, AfterSchema: 2, BeforePackageSHA256: strings.Repeat("a", 64), AfterPackageSHA256: strings.Repeat("b", 64), Before: document.Values["example.tool"], After: json.RawMessage(`{"size":3,"label":"migrated"}`)}
	return store, snapshot
}

func TestPackageMigrationPersistsVerifiableBeforeAndAfterWithoutApplying(t *testing.T) {
	store, snapshot := migrationFixture(t)
	digest, err := store.SavePackageMigration(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.LoadPackageMigration(snapshot.OperationID, digest)
	if err != nil {
		t.Fatal(err)
	}
	if string(recovered.Before) != `{"size":3}` || string(recovered.After) != `{"size":3,"label":"migrated"}` {
		t.Fatal("snapshot lost configuration")
	}
	if string(store.CommittedDocument().Values["example.tool"]) != `{"size":3}` {
		t.Fatal("preparing migration changed committed settings")
	}
	recovered.Before[0] = '['
	if again, err := store.LoadPackageMigration(snapshot.OperationID, digest); err != nil || string(again.Before) != `{"size":3}` {
		t.Fatal("caller changed stored snapshot")
	}
	if _, err = store.SavePackageMigration(snapshot); err == nil {
		t.Fatal("snapshot identity was overwritten")
	}
	path := filepath.Join(filepath.Dir(store.store.path), "package-migrations", "migration-"+snapshot.OperationID+".json")
	if err = os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.LoadPackageMigration(snapshot.OperationID, digest); err == nil {
		t.Fatal("changed migration snapshot accepted")
	}
}

func TestPackageMigrationDigestCanBeRecordedBeforeWritingBackup(t *testing.T) {
	store, snapshot := migrationFixture(t)
	snapshot.After = json.RawMessage("{\n\"size\":3, \"label\":\"migrated\"\n}")
	expected, err := PackageMigrationDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := store.packageMigrationPath(snapshot.OperationID)
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("digest wrote a recovery file", err)
	}
	actual, err := store.SavePackageMigration(snapshot)
	if err != nil || actual != expected {
		t.Fatal("saved bytes differ from the preparation identity", actual, expected, err)
	}
	if _, err = store.LoadPackageMigration(snapshot.OperationID, expected); err != nil {
		t.Fatal(err)
	}
	snapshot.After = json.RawMessage(`{"bad":`)
	if _, err = PackageMigrationDigest(snapshot); err == nil {
		t.Fatal("invalid backup obtained an identity")
	}
}

func TestPackageMigrationRequiresIsolationAndRollbackPreservesOtherChanges(t *testing.T) {
	store, snapshot := migrationFixture(t)
	if err := store.ApplyPackageMigration(snapshot, true, func(json.RawMessage) error { return nil }); err == nil {
		t.Fatal("live module configuration changed outside quarantine")
	}
	if err := store.QuarantinePackage(snapshot.ModuleID, errors.New("updating")); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyPackageMigration(snapshot, true, func(raw json.RawMessage) error {
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		if value["label"] != "migrated" {
			return errors.New("new schema requires label")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	global := store.Snapshot()
	global.Theme = "dark"
	if _, err := store.Save(global); err != nil {
		t.Fatal("unrelated settings became blocked", err)
	}
	if err := store.ApplyPackageMigration(snapshot, false, packageAdapter().Validate); err != nil {
		t.Fatal(err)
	}
	document := store.CommittedDocument()
	if string(document.Values[snapshot.ModuleID]) != `{"size":3}` || store.Snapshot().Theme != "dark" {
		t.Fatal("rollback lost old module settings or unrelated changes")
	}
}

func TestPackageMigrationDoesNotDiscardUnrelatedPendingConfiguration(t *testing.T) {
	store, snapshot := migrationFixture(t)
	if err := store.QuarantinePackage(snapshot.ModuleID, errors.New("updating")); err != nil {
		t.Fatal(err)
	}
	current := store.CommittedDocument()
	pending := clone(current)
	pending.Revision++
	var global map[string]any
	if err := json.Unmarshal(pending.Values["host"], &global); err != nil {
		t.Fatal(err)
	}
	global["theme"] = "dark"
	pending.Values["host"], _ = json.Marshal(global)
	if err := store.store.persist(diskState{Committed: current, Pending: &pending}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyPackageMigration(snapshot, false, packageAdapter().Validate); err == nil || !store.store.NeedsRecovery() {
		t.Fatal("unrelated pending transaction was discarded")
	}
}

func TestPackageMigrationValidatesRollbackBeforeClearingPendingData(t *testing.T) {
	store, snapshot := migrationFixture(t)
	if err := store.QuarantinePackage(snapshot.ModuleID, errors.New("updating")); err != nil {
		t.Fatal(err)
	}
	current := store.CommittedDocument()
	pending := clone(current)
	pending.Revision++
	pending.Values[snapshot.ModuleID] = snapshot.After
	if err := store.store.persist(diskState{Committed: current, Pending: &pending}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyPackageMigration(snapshot, false, func(json.RawMessage) error { return errors.New("old definition rejected") }); err == nil || !store.store.NeedsRecovery() {
		t.Fatal("failed rollback validation discarded the pending transaction")
	}
}

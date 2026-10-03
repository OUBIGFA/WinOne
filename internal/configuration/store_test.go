package configuration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func defaults() Document {
	return Document{SchemaVersion: 1, Values: map[string]json.RawMessage{"host": json.RawMessage(`{"theme":"light"}`)}}
}
func valid(Document) error { return nil }

func TestCommitPersistsOnlyAfterRuntimeApplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	store, err := Open(path, defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	next := store.Snapshot()
	next.Values["host"] = json.RawMessage(`{"theme":"dark"}`)
	_, err = store.Commit(next, func(target Document) error {
		reopened, err := Open(path, defaults(), valid)
		if err != nil {
			return err
		}
		if !reopened.NeedsRecovery() || string(reopened.Snapshot().Values["host"]) != `{"theme":"light"}` {
			t.Fatal("uncommitted state exposed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.NeedsRecovery() || reopened.Snapshot().Revision != 1 || string(reopened.Snapshot().Values["host"]) != `{"theme":"dark"}` {
		t.Fatal(reopened.Snapshot())
	}
	if _, err = reopened.Commit(next, func(Document) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("stale write accepted", err)
	}
}

func TestFailedApplyRestoresPreviousDeviceState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "configuration.json")
	device := filepath.Join(root, "device-state.json")
	store, err := Open(path, defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	next := store.Snapshot()
	next.Values["host"] = json.RawMessage(`{"theme":"dark"}`)
	_, err = store.Commit(next, func(target Document) error {
		if err := os.WriteFile(device, target.Values["host"], 0600); err != nil {
			return err
		}
		if string(target.Values["host"]) == `{"theme":"dark"}` {
			return errors.New("second module failed")
		}
		return nil
	})
	if err == nil {
		t.Fatal("failed apply reported success")
	}
	actual, err := os.ReadFile(device)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != `{"theme":"light"}` || store.NeedsRecovery() || store.Snapshot().Revision != 0 {
		t.Fatal(string(actual))
	}
}

func TestInterruptedCommitRequiresRecoveryBeforeNewWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	store, err := Open(path, defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	next := store.Snapshot()
	next.Values["host"] = json.RawMessage(`{"theme":"dark"}`)
	_, err = store.Commit(next, func(Document) error { return errors.New("device unavailable") })
	if err == nil || !store.NeedsRecovery() {
		t.Fatal("failed rollback not retained")
	}
	store, err = Open(path, defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Commit(next, func(Document) error { return nil }); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal(err)
	}
	var recovered Document
	if err = store.Recover(func(target Document) error { recovered = target; return nil }); err != nil {
		t.Fatal(err)
	}
	if string(recovered.Values["host"]) != `{"theme":"light"}` || store.NeedsRecovery() {
		t.Fatal(recovered)
	}
	store, err = Open(path, defaults(), valid)
	if err != nil || store.NeedsRecovery() {
		t.Fatal(err)
	}
}

func TestSnapshotsAndCallbacksCannotMutateCommittedConfiguration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "configuration.json"), defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.Snapshot()
	snapshot.Values["host"][2] = 'X'
	next := store.Snapshot()
	_, err = store.Commit(next, func(target Document) error { target.Values["host"][2] = 'Y'; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if string(store.Snapshot().Values["host"]) != `{"theme":"light"}` {
		t.Fatal("shared mutable configuration")
	}
}

func TestConcurrentCommitCannotOverwriteNewRevision(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "configuration.json"), defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	next := store.Snapshot()
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := store.Commit(next, func(Document) error { close(entered); <-release; return nil })
		first <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("apply did not start")
	}
	second := make(chan error, 1)
	go func() { _, err := store.Commit(next, func(Document) error { return nil }); second <- err }()
	observed := make(chan Document, 1)
	go func() { observed <- store.Snapshot() }()
	select {
	case current := <-observed:
		if current.Revision != 0 {
			t.Fatal("exposed pending revision")
		}
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("read blocked by apply")
	}
	close(release)
	if err = <-first; err != nil {
		t.Fatal(err)
	}
	if err = <-second; !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestInvalidDiskStateDoesNotFallBackToDefaults(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"committed":{"schemaVersion":1,"revision":0,"values":null}}`, `{"committed":{"schemaVersion":1,"revision":0,"values":{"host":{}}},"pending":{"schemaVersion":1,"revision":4,"values":{"host":{}}}}`} {
		path := filepath.Join(t.TempDir(), "configuration.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path, defaults(), valid); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestFailedPreparationDoesNotTouchRuntime(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "not-a-directory")
	store, err := Open(filepath.Join(parent, "configuration.json"), defaults(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(parent, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	device := filepath.Join(root, "device-state.json")
	_, err = store.Commit(store.Snapshot(), func(target Document) error { return os.WriteFile(device, target.Values["host"], 0600) })
	if err == nil || store.NeedsRecovery() {
		t.Fatal("failed prepare was accepted")
	}
	if _, err = os.Stat(device); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("runtime changed before journal", err)
	}
}

func TestDuplicateDiskFieldsAreRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.json")
	raw := `{"committed":{"schemaVersion":1,"revision":0,"revision":1,"values":{"host":{}}}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, defaults(), valid); err == nil {
		t.Fatal("duplicate revision accepted")
	}
}

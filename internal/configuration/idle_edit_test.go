package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func idleModules(t *testing.T) *HostStore {
	t.Helper()
	adapters := []ModuleAdapter{}
	for _, id := range []string{"capture", "selection", "quick-assistant", "always-on-top", "stroke", "timed-power", "wintray"} {
		adapters = append(adapters, ModuleAdapter{ID: id,
			Load:     func() (json.RawMessage, error) { return json.RawMessage(`{"size":1}`), nil },
			Validate: func(raw json.RawMessage) error { var value struct{ Size int }; return json.Unmarshal(raw, &value) },
			Apply:    func(json.RawMessage, uint64) error { return nil },
		})
	}
	store, err := OpenHost(t.TempDir(), adapters...)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestIdleModuleEditsSurviveUnrelatedSaves(t *testing.T) {
	store := idleModules(t)
	original := store.CommittedDocument()
	globalDraft := store.Snapshot()
	globalDraft.Theme = "dark"
	for _, id := range []string{"capture", "selection", "quick-assistant", "always-on-top", "stroke", "timed-power", "wintray"} {
		if _, _, err := store.SaveModule(id, original.Revision, json.RawMessage(`{"size":2}`)); err != nil {
			t.Fatalf("idle %s editor was invalidated by another module: %v", id, err)
		}
	}
	if _, err := store.Save(globalDraft); err != nil {
		t.Fatal("module changes invalidated global preferences", err)
	}
	for id := range original.Values {
		if id == "host" {
			continue
		}
		if _, _, err := store.SaveModule(id, original.Revision, json.RawMessage(`{"size":3}`)); !errors.Is(err, ErrConflict) {
			t.Fatalf("same-module conflict was not protected: %s %v", id, err)
		}
	}
	if _, err := store.CommitDocument(original); !errors.Is(err, ErrConflict) {
		t.Fatal("whole profile lost revision protection", err)
	}
}

func TestIdleModuleEditHasNoRevisionAgeLimit(t *testing.T) {
	store := idleModules(t)
	_, original, _ := store.Module("selection")
	for i := 0; i < 40; i++ {
		_, revision, _ := store.Module("capture")
		if _, _, err := store.SaveModule("capture", revision, json.RawMessage(fmt.Sprintf(`{"size":%d}`, i+2))); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.SaveModule("selection", original, json.RawMessage(`{"size":7}`)); err != nil {
		t.Fatal("old untouched page expired", err)
	}
}

func TestConcurrentIndependentModuleEditsAreSerialized(t *testing.T) {
	store := idleModules(t)
	revision := store.CommittedDocument().Revision
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"capture", "selection"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.SaveModule(id, revision, json.RawMessage(`{"size":8}`))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("independent writer was rejected", err)
		}
	}
	for _, id := range []string{"capture", "selection"} {
		raw, _, err := store.Module(id)
		if err != nil || string(raw) != `{"size":8}` {
			t.Fatal("write lost", id, err)
		}
	}
}

func TestFailedProjectionDoesNotInvalidateIdlePage(t *testing.T) {
	store := idleModules(t)
	_, original, _ := store.Module("capture")
	store.modules[0].Apply = func(raw json.RawMessage, _ uint64) error {
		if string(raw) == `{"size":9}` {
			return errors.New("fixture failure")
		}
		return nil
	}
	if _, _, err := store.SaveModule("capture", original, json.RawMessage(`{"size":9}`)); err == nil {
		t.Fatal("fixture should fail")
	}
	if _, _, err := store.SaveModule("capture", original, json.RawMessage(`{"size":3}`)); err != nil {
		t.Fatal("rollback invalidated page", err)
	}
}

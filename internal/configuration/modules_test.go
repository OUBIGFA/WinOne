package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestModuleConfigurationMigratesOnceAndCompensates(t *testing.T) {
	root := t.TempDir()
	runtime := json.RawMessage(`{"size":3}`)
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(raw json.RawMessage) error { var value struct{ Size int }; return json.Unmarshal(raw, &value) }, Apply: func(raw json.RawMessage, _ uint64) error {
		runtime = raw
		if string(raw) == `{"size":9}` {
			return errors.New("apply failed")
		}
		return nil
	}}
	store, err := OpenHost(root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := store.Module(adapter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.SaveModule(adapter.ID, revision, json.RawMessage(`{"size":9}`)); err == nil || string(runtime) != `{"size":3}` {
		t.Fatal(string(runtime), err)
	}
	if _, _, err = store.SaveModule(adapter.ID, revision, json.RawMessage(`{"size":5}`)); err != nil {
		t.Fatal(err)
	}
	adapter.Load = func() (json.RawMessage, error) { return nil, errors.New("legacy unavailable") }
	store, err = OpenHost(root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := store.Module(adapter.ID)
	if err != nil || string(raw) != `{"size":5}` {
		t.Fatal(string(raw), err)
	}
}

func TestLegacyModuleFailureDoesNotBlockHostPreferences(t *testing.T) {
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return nil, errors.New("broken module") }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return errors.New("must not apply missing module") }}
	store, err := OpenHost(t.TempDir(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Module(adapter.ID); err == nil {
		t.Fatal("missing module treated as ready")
	}
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); err != nil {
		t.Fatal(err)
	}
}

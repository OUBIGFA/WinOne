package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestLifecycleCommitCompensatesEnabledStateAndPreferencesTogether(t *testing.T) {
	root := t.TempDir()
	enabled := true
	projection := `{"size":3}`
	lifecycle := ModuleLifecycle{
		Initial: map[string]bool{"example.tool": true},
		Before: func(target map[string]bool) error {
			if !target["example.tool"] {
				enabled = false
			}
			return nil
		},
		After: func(target map[string]bool) error {
			if target["example.tool"] {
				enabled = true
			}
			return nil
		},
	}
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(projection), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(raw json.RawMessage, _ uint64) error {
		projection = string(raw)
		if projection == `{"size":9}` {
			return errors.New("native apply failed")
		}
		return nil
	}}
	store, err := OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled["example.tool"] = false
	next.Values["example.tool"] = json.RawMessage(`{"size":9}`)
	if _, err = store.CommitDocument(next); err == nil {
		t.Fatal("failed transaction accepted")
	}
	if !enabled || projection != `{"size":3}` || !store.CommittedDocument().ModuleEnabled["example.tool"] {
		t.Fatal("state was only partially restored")
	}
	next = store.CommittedDocument()
	next.ModuleEnabled["example.tool"] = false
	next.Values["example.tool"] = json.RawMessage(`{"size":5}`)
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil || reopened.CommittedDocument().ModuleEnabled["example.tool"] || enabled || projection != `{"size":5}` {
		t.Fatal("restart changed committed intent", err)
	}
	if _, err = OpenHost(root, adapter); err == nil {
		t.Fatal("recovery opened without lifecycle support")
	}
}

func TestEnableRunsAfterConfigurationProjection(t *testing.T) {
	enabled := false
	projected := ""
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(raw json.RawMessage, _ uint64) error { projected = string(raw); return nil }}
	lifecycle := ModuleLifecycle{Initial: map[string]bool{"example.tool": false}, Before: func(map[string]bool) error { return nil }, After: func(target map[string]bool) error {
		if target["example.tool"] {
			if projected != `{"size":5}` {
				return errors.New("enabled before new settings were projected")
			}
			enabled = true
		}
		return nil
	}}
	store, err := OpenHostWithLifecycle(t.TempDir(), lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled["example.tool"] = true
	next.Values["example.tool"] = json.RawMessage(`{"size":5}`)
	if _, err = store.CommitDocument(next); err != nil || !enabled {
		t.Fatal(err)
	}
	snapshot := store.CommittedDocument()
	snapshot.ModuleEnabled["example.tool"] = false
	if !store.CommittedDocument().ModuleEnabled["example.tool"] {
		t.Fatal("caller mutated committed switches")
	}
}

func TestRestartRecoversBothSwitchesAndPreferencesAfterFailedCompensation(t *testing.T) {
	root := t.TempDir()
	fail := false
	enabled := true
	projection := ""
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(raw json.RawMessage, _ uint64) error { projection = string(raw); return nil }}
	lifecycle := ModuleLifecycle{Initial: map[string]bool{"example.tool": true}, Before: func(target map[string]bool) error {
		if !target["example.tool"] {
			enabled = false
		}
		return nil
	}, After: func(target map[string]bool) error {
		if fail {
			return errors.New("lifecycle unavailable")
		}
		enabled = target["example.tool"]
		return nil
	}}
	store, err := OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled["example.tool"] = false
	next.Values["example.tool"] = json.RawMessage(`{"size":5}`)
	fail = true
	if _, err = store.CommitDocument(next); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal(err)
	}
	if _, err = store.CommitDocument(next); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("new edit bypassed recovery", err)
	}
	fail = false
	store, err = OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || projection != `{"size":3}` || store.CommittedDocument().Revision != 0 || !store.CommittedDocument().ModuleEnabled["example.tool"] {
		t.Fatal("restart promoted or partially restored pending settings")
	}
}

func TestStartupRecoveryFailureKeepsCommittedSettingsReadable(t *testing.T) {
	fail := true
	lifecycle := ModuleLifecycle{Initial: map[string]bool{"example.tool": false}, Before: func(map[string]bool) error {
		if fail {
			return errors.New("native recovery unavailable")
		}
		return nil
	}, After: func(map[string]bool) error { return nil }}
	store, err := OpenHostWithLifecycle(t.TempDir(), lifecycle)
	if err == nil || store == nil || store.RecoveryError() == nil {
		t.Fatal("startup recovery failure hid its readable state", err)
	}
	if store.Snapshot().Theme != "system" {
		t.Fatal(store.Snapshot())
	}
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("edited during recovery", err)
	}
	fail = false
	if err = store.Recover(); err != nil || store.RecoveryError() != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(next); err != nil {
		t.Fatal(err)
	}
}

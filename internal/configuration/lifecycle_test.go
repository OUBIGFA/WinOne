package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSettledObserverRestoresReadinessAfterCommitAndRecovery(t *testing.T) {
	var store *HostStore
	enabled := false
	failure := ""
	duringApply := false
	adapter := ModuleAdapter{ID: "capture", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }}
	lifecycle := ModuleLifecycle{
		Initial: map[string]bool{"capture": false},
		Before:  func(map[string]bool) error { return nil },
		After: func(target map[string]bool) error {
			enabled = target["capture"]
			if store != nil {
				// This is the Host's original readiness check, invoked during
				// module intent application while the journal is still pending.
				duringApply = store.RecoveryError() == nil && enabled
			}
			if failure == "recovery" || failure == "rollback" && !enabled {
				return errors.New("native transition failed")
			}
			return nil
		},
	}
	var err error
	store, err = OpenHostWithLifecycle(t.TempDir(), lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	var states []bool
	store.OnSettled(func() { states = append(states, store.RecoveryError() == nil && enabled) })
	next := store.CommittedDocument()
	next.ModuleEnabled["capture"] = true
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal(err)
	}
	if duringApply || len(states) != 1 || !states[0] {
		t.Fatalf("readiness was not restored after the pending journal cleared: during=%v, settled=%v", duringApply, states)
	}
	failure = "rollback"
	next = store.CommittedDocument()
	next.ModuleEnabled["capture"] = false
	if _, err = store.CommitDocument(next); err == nil || len(states) != 2 || !states[1] {
		t.Fatalf("clean rollback did not restore readiness: %v %v", err, states)
	}
	failure = "recovery"
	if _, err = store.CommitDocument(next); err == nil || len(states) != 3 || states[2] {
		t.Fatalf("incomplete recovery retained readiness: %v %v", err, states)
	}
	failure = ""
	if err = store.Recover(); err != nil || len(states) != 4 || !states[3] {
		t.Fatalf("explicit recovery did not restore readiness: %v %v", err, states)
	}
}

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

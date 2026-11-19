package modules

import (
	"errors"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modulestate"
)

func TestEnabledModuleCanRetryAFaultedRuntime(t *testing.T) {
	store, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := "faulted"
	gate, err := NewGate("capture", "Capture", store, LifecycleHooks{
		Start:         func() error { state = "ready"; return nil },
		Stop:          func() error { return nil },
		IntentChanged: func() error { return nil },
		Status:        func() host.Module { return host.Module{ID: "capture", State: state} },
		Handle:        func(r host.Request) host.Response { return host.Response{ID: r.ID} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(store.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if got := gate.Status(); got.State != "ready" {
		t.Fatalf("saved enablement prevented runtime recovery: %+v", got)
	}
}

func TestBusyStopRefusalPreservesIntentAndAllowsLaterDisable(t *testing.T) {
	store, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	busy := true
	stops, intents := 0, 0
	gate, err := NewGate("capture", "Capture", store, LifecycleHooks{
		Start: func() error { return nil }, Stop: func() error { stops++; return nil },
		CanStop: func() error {
			if busy {
				return errors.New("editor is open")
			}
			return nil
		},
		IntentChanged: func() error { intents++; return nil },
		Status:        func() host.Module { return host.Module{State: "ready"} },
		Handle:        func(r host.Request) host.Response { return host.Response{ID: r.ID} },
	})
	if err != nil {
		t.Fatal(err)
	}
	revision := store.Snapshot().Revision
	if err := gate.SetEnabled(revision, false); err == nil {
		t.Fatal("active editor was not protected")
	}
	record := store.Snapshot().Modules["capture"]
	if !record.Enabled || record.PendingStop || store.Snapshot().Revision != revision || intents != 0 || stops != 0 || gate.Status().State != "ready" {
		t.Fatalf("refusal changed lifecycle state: %+v, stops=%d intents=%d", record, stops, intents)
	}
	busy = false
	if err := gate.SetEnabled(revision, false); err != nil || gate.Status().State != "disabled" || stops != 1 {
		t.Fatal("finished editor could not disable normally", err)
	}
}

func TestFailedStopBlocksCommandsAndRequiresRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.json")
	store, err := modulestate.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	failStop := true
	commands, starts, stops, intents := 0, 0, 0, 0
	hooks := LifecycleHooks{
		Start: func() error { starts++; return nil },
		Stop: func() error {
			stops++
			if failStop {
				return errors.New("window restoration failed")
			}
			return nil
		},
		IntentChanged: func() error {
			intents++
			if store.Snapshot().Modules["capture"].Enabled && starts == 0 && stops == 0 {
				t.Fatal("intent hook ran before persistence")
			}
			return nil
		},
		Status: func() host.Module { return host.Module{ID: "capture", State: "ready"} },
		Handle: func(r host.Request) host.Response { commands++; return host.Response{ID: r.ID} },
	}
	gate, err := NewGate("capture", "截图", store, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(store.Snapshot().Revision, false); err == nil {
		t.Fatal("failed restoration accepted")
	}
	if gate.Status().State != "faulted" {
		t.Fatal("failed stop displayed as disabled")
	}
	if r := gate.Handle(host.Request{Version: 1, ID: "one", Method: "capture.start"}); r.Error == nil || commands != 0 {
		t.Fatal("disabled module still executed an action")
	}
	restarted, err := modulestate.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	gate, err = NewGate("capture", "截图", restarted, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(restarted.Snapshot().Revision, true); err == nil {
		t.Fatal("pending cleanup was bypassed")
	}
	failStop = false
	if err = gate.SetEnabled(restarted.Snapshot().Revision, false); err != nil {
		t.Fatal(err)
	}
	if gate.Status().State != "disabled" {
		t.Fatal("successful stop not disabled")
	}
	beforeStops := stops
	if err = gate.SetEnabled(restarted.Snapshot().Revision, false); err != nil || stops != beforeStops {
		t.Fatal("duplicate disable repeated native effects")
	}
	if err = gate.SetEnabled(restarted.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if r := gate.Handle(host.Request{Version: 1, ID: "two", Method: "capture.start"}); r.Error != nil || commands != 1 || starts != 1 {
		t.Fatal("enabled module did not resume")
	}
	if intents != 3 {
		t.Fatalf("unexpected intent updates: %d", intents)
	}
}

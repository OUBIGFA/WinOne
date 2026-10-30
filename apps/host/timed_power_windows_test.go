//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/timedpower"
)

func TestTimedPowerConfigurationDoesNotArm(t *testing.T) {
	store, err := configuration.OpenHost(t.TempDir(), timedPowerConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	handler := timedPowerConfigurationHandler(store, func(r host.Request) host.Response { t.Fatal("settings activated module"); return host.Response{} })
	reply := handler(host.Request{ID: "get", Method: "settings.get"})
	var p timedpower.Preferences
	if reply.Error != nil || json.Unmarshal(reply.Result, &p) != nil || !p.Force {
		t.Fatal(reply)
	}
	p.DelaySeconds = 90
	raw, _ := json.Marshal(p)
	reply = handler(host.Request{ID: "save", Method: "settings.save", Params: raw})
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	reply = handler(host.Request{ID: "stale", Method: "settings.save", Params: raw})
	if reply.Error == nil {
		t.Fatal("stale revision accepted")
	}
}
func TestTimedPowerPeerAndDisabledGate(t *testing.T) {
	m := timedpower.New(func(timedpower.Preferences) error { t.Fatal("native execution reached"); return nil }, nil)
	defer m.Close()
	handler := timedPowerHandler(m, filepath.Join(t.TempDir(), "WinOne.exe"))
	p := timedpower.Defaults()
	p.DryRun = true
	p.DelaySeconds = 600
	raw, _ := json.Marshal(map[string]any{"preferences": p, "confirmed": true})
	reply := handler(host.Request{ID: "foreign", PeerPID: uint32(os.Getpid()), Method: "start", Params: raw})
	if reply.Error == nil || m.Snapshot().Active {
		t.Fatal("foreign peer armed a task")
	}
	// Validated same-executable identity in a test-only handler; still reminder-only.
	executable, _ := os.Executable()
	handler = timedPowerHandler(m, executable)
	states, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := modules.NewGate("timed-power", "定时关机", states, modules.LifecycleHooks{Start: func() error { return nil }, Stop: m.Close, Status: func() host.Module { return timedPowerStatus(m) }, Handle: handler, IntentChanged: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	reply = gate.Handle(host.Request{ID: "own", PeerPID: uint32(os.Getpid()), Method: "start", Params: raw})
	if reply.Error != nil || !m.Snapshot().Active {
		t.Fatal(reply)
	}
	if err = gate.SetEnabled(states.Snapshot().Revision, false); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Active || gate.Handle(host.Request{ID: "disabled", Method: "start", Params: raw}).Error == nil {
		t.Fatal("disabled gate accepted action")
	}
	if err = gate.SetEnabled(states.Snapshot().Revision, true); err != nil || m.Snapshot().Active {
		t.Fatal("enable resumed timer", err)
	}
}

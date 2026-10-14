//go:build windows

package traybox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTargetPinsTheIconOwner(t *testing.T) {
	fixture := startFakeShell(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	icon := Icon{owner: uint32(fixture.program), ExePath: executable, callback: testCallback}
	target, err := openIconTarget(icon)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	states, err := snapshotOpenTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, state := range states {
		if state.handle == uintptr(fixture.program) {
			found = true
		}
	}
	if !found {
		t.Fatal("Pinned target lost its window")
	}
	if _, err := snapshotOpenTarget(nil); err == nil {
		t.Fatal("Unpinned snapshot accepted")
	}
	if target.PID != uint32(os.Getpid()) {
		t.Fatal("Wrong target process")
	}
	wrong := icon
	wrong.ExePath = filepath.Join(t.TempDir(), "other.exe")
	if p, err := openIconTarget(wrong); err == nil {
		p.Close()
		t.Fatal("Mismatched executable accepted")
	}
	missing := icon
	missing.owner = 0
	if p, err := openIconTarget(missing); err == nil {
		p.Close()
		t.Fatal("Missing owner accepted")
	}
}

func TestNativeOpenWindowSnapshotIncludesOnlyTarget(t *testing.T) {
	fixture := startFakeShell(t)
	states, err := snapshotOpenWindows(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, state := range states {
		if state.pid != uint32(os.Getpid()) {
			t.Fatal("Unrelated process included")
		}
		if state.handle == uintptr(fixture.program) {
			found = true
			if state.visible {
				t.Fatal("Hidden fixture reported visible")
			}
		}
	}
	if !found {
		t.Fatal("Fixture missing from snapshot")
	}
	if _, err := snapshotOpenWindows(0); err == nil {
		t.Fatal("Missing identity accepted")
	}
}

func TestOpenOutcomeRequiresTargetWindowTransition(t *testing.T) {
	before := []openWindowState{{handle: 1, pid: 7, visible: true}, {handle: 2, pid: 7, visible: false}}
	for _, tc := range []struct {
		name  string
		after []openWindowState
		want  bool
	}{
		{"unchanged", before, false},
		{"unrelated process", []openWindowState{{handle: 3, pid: 9, visible: true, foreground: true}}, false},
		{"restored hidden window", []openWindowState{{handle: 2, pid: 7, visible: true}}, true},
		{"new window", []openWindowState{{handle: 3, pid: 7, visible: true}}, true},
		{"foreground transition", []openWindowState{{handle: 1, pid: 7, visible: true, foreground: true}}, true},
		{"minimized", []openWindowState{{handle: 3, pid: 7, visible: true, minimized: true}}, false},
		{"cloaked", []openWindowState{{handle: 3, pid: 7, visible: true, cloaked: true}}, false},
		{"hidden", []openWindowState{{handle: 3, pid: 7}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := observedOpen(before, tc.after, 7); got != tc.want {
				t.Fatalf("observed=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestOpenOutcomeAlreadyForegroundIsNotNewSuccess(t *testing.T) {
	state := []openWindowState{{handle: 1, pid: 7, visible: true, foreground: true}}
	if observedOpen(state, state, 7) {
		t.Fatal("An unchanged foreground window is not evidence of a new activation")
	}
	if observedOpen(nil, state, 0) {
		t.Fatal("Missing target identity accepted")
	}
}

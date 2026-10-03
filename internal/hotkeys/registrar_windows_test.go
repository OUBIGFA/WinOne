//go:build windows

package hotkeys

import (
	"os"
	"testing"
	"time"
)

// Opt-in tests reserve a rare chord without injecting input into user applications.
func TestNativeRegistrationConflictAndRelease(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_HOTKEY_TEST") != "1" {
		t.Skip("set WINONE_NATIVE_HOTKEY_TEST=1")
	}
	first, second := New(), New()
	defer first.Close()
	defer second.Close()
	binding := []Binding{{"test", "Ctrl+Alt+Shift+F23"}}
	statuses, err := first.Update(binding)
	if err != nil || !statuses[0].Registered {
		t.Fatalf("first registration: %+v %v", statuses, err)
	}
	statuses, err = second.Update(binding)
	if err != nil || statuses[0].Registered || statuses[0].Error == "" {
		t.Fatalf("conflict: %+v %v", statuses, err)
	}
	if _, err = first.Update([]Binding{{"test", ""}}); err != nil {
		t.Fatal(err)
	}
	statuses, err = second.Update(binding)
	if err != nil || !statuses[0].Registered {
		t.Fatalf("retry: %+v %v", statuses, err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	statuses, err = first.Update(binding)
	if err != nil || !statuses[0].Registered {
		t.Fatalf("release: %+v %v", statuses, err)
	}
}

func TestQueuedOldHotkeyDoesNotTriggerReplacement(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_HOTKEY_TEST") != "1" {
		t.Skip("set WINONE_NATIVE_HOTKEY_TEST=1")
	}
	r := New()
	defer r.Close()
	for _, binding := range []Binding{{"old", "Ctrl+Alt+Shift+F23"}, {"new", "Ctrl+Alt+Shift+F24"}} {
		states, err := r.Update([]Binding{binding})
		if err != nil || !states[0].Registered {
			t.Fatalf("registration: %+v %v", states, err)
		}
	}
	// Model delayed Windows delivery without injecting keyboard input into the desktop.
	if ok, _, err := post.Call(uintptr(r.thread), 0x312, 1, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case id := <-r.Events():
		t.Fatalf("old event invoked %s", id)
	case <-time.After(100 * time.Millisecond):
	}
	if ok, _, err := post.Call(uintptr(r.thread), 0x312, 2, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case id := <-r.Events():
		if id != "new" {
			t.Fatalf("unexpected %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("new binding did not dispatch")
	}
}

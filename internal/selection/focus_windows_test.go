//go:build windows

package selection

import "testing"

func TestLateFocusNotificationDoesNotDismissNewSelection(t *testing.T) {
	w := &Watcher{events: make(chan Event, 1)}
	w.emit(Event{Kind: "selection", Window: 100, ProcessID: 10})
	selected := w.sequence.Load()
	w.emitFocus(100, 100)
	if !w.Current(selected) {
		t.Fatal("same-window focus canceled a valid selection")
	}
	if event := <-w.events; event.Kind != "selection" {
		t.Fatal("toolbar dismissed", event)
	}
	w.emitFocus(200, 200)
	if w.Current(selected) {
		t.Fatal("real foreground change did not cancel selection")
	}
	changed := w.sequence.Load()
	w.emitFocus(100, 200)
	if !w.Current(changed) {
		t.Fatal("stale foreground event changed current state")
	}
}

func TestPointerDownUsesWindowUnderPointerBeforeActivation(t *testing.T) {
	// Low-level hooks run before Windows activates the clicked window.
	if got := pointerSource(100, 200, true); got != 200 {
		t.Fatalf("source=%d", got)
	}
	if got := pointerSource(100, 200, false); got != 100 {
		t.Fatalf("mouse-up source=%d", got)
	}
	if got := pointerSource(100, 0, true); got != 100 {
		t.Fatalf("missing hit source=%d", got)
	}
}

func TestKeyboardSelectionSurvivesModifierReleasedFirst(t *testing.T) {
	var g KeyboardGesture
	g.Key(true, 0x10, true, false, false)
	g.Key(true, 0x27, true, false, false)
	if g.Key(false, 0x10, false, false, false) {
		t.Fatal("selection key still down")
	}
	if !g.Key(false, 0x27, false, false, false) {
		t.Fatal("lost Shift selection on release")
	}
	g.Key(true, 'A', false, true, false)
	g.Key(false, 0x11, false, false, false)
	if !g.Key(false, 'A', false, false, false) {
		t.Fatal("lost select all on release")
	}
	if g.Key(false, 'C', false, false, false) {
		t.Fatal("ordinary key became selection")
	}
}

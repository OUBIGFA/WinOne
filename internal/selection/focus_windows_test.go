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

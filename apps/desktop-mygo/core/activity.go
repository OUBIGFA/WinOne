package core

// Activity collects what the current page is doing, by source: a draft
// with unsaved changes (dirty) or an operation still running (busy). The
// shell reads it before leaving the page, switching a tab or closing the
// window, as the React window's usePageNavigation did.
//
// Pages report on every build, from their page-level state, so a tab that
// is not shown still counts: its draft lives in the page, not the view.
type Activity struct {
	sections map[string]section
}

type section struct {
	dirty, busy bool
	discard     func()
}

// Report records the activity of source. discard reverts its draft in
// place; it is called when the user chooses to abandon changes while the
// page stays, as on a tab switch.
func (a *Activity) Report(source string, dirty, busy bool, discard func()) {
	if a.sections == nil {
		a.sections = map[string]section{}
	}
	if !dirty && !busy {
		delete(a.sections, source)
		return
	}
	a.sections[source] = section{dirty: dirty, busy: busy, discard: discard}
}

// Reset forgets every source, as the shell shows another page.
func (a *Activity) Reset() { clear(a.sections) }

// State returns whether any source is dirty or busy.
func (a *Activity) State() (dirty, busy bool) {
	for _, s := range a.sections {
		dirty = dirty || s.dirty
		busy = busy || s.busy
	}
	return dirty, busy
}

// Discard reverts every dirty source that can revert in place.
func (a *Activity) Discard() {
	for _, s := range a.sections {
		if s.dirty && s.discard != nil {
			s.discard()
		}
	}
}

// Rail is the page's entry in the shell's action rail: Save and Undo,
// always shown at the same place, disabled when the page sets none.
// The shell clears it before building the page each frame.
type Rail struct {
	Save, Undo Action
	active     bool
}

// Action is one button of the rail.
type Action struct {
	// Label names the action; for Save it is the transaction's own name,
	// as "保存翻译设置".
	Label    string
	Disabled bool
	Busy     bool
	Run      func()
}

// Set shows the page's Save and Undo. Undo with a nil Run stays disabled.
func (r *Rail) Set(save, undo Action) { r.Save, r.Undo, r.active = save, undo, true }

// Active reports whether the page set an action this frame.
func (r *Rail) Active() bool { return r.active }

// Clear removes the page's actions; the shell calls it before each build.
func (r *Rail) Clear() { *r = Rail{} }

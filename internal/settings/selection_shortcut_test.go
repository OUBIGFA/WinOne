package settings

import "testing"

func TestSelectionShortcutAdoptsDefaultWithoutConflictingOrChangingExplicitChoice(t *testing.T) {
	old := Settings{SchemaVersion: 1, Theme: "system", CaptureShortcut: "Alt+Shift+S"}
	if old.SelectionBinding() != "" {
		t.Fatal("new default conflicted with old binding")
	}
	old.CaptureShortcut = "Ctrl+Alt+Z"
	if old.SelectionBinding() != "Alt+Shift+S" {
		t.Fatal("default selection shortcut missing")
	}
	empty := ""
	old.SelectionShortcut = &empty
	resolved := old.ResolvedShortcuts()
	*resolved.SelectionShortcut = "Alt+Shift+X"
	if old.SelectionBinding() != "" {
		t.Fatal("snapshot changed disabled selection shortcut")
	}
}

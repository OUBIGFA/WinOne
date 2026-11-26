package captureprefs

import "testing"

func TestCanvasShortcutsPreserveDefaultsAndRejectConflicts(t *testing.T) {
	if err := ValidateCanvasShortcuts(nil); err != nil {
		t.Fatal(err)
	}
	if CanvasShortcut(nil, "number") != "N" {
		t.Fatal("number default lost")
	}
	if CanvasShortcut(map[string]string{"number": ""}, "number") != "" {
		t.Fatal("disabled key restored")
	}
	if ValidateCanvasShortcuts(map[string]string{"number": "R"}) == nil {
		t.Fatal("conflicting rectangle key accepted")
	}
	if err := ValidateCanvasShortcuts(map[string]string{"number": "F8", "rectangle": ""}); err != nil {
		t.Fatal(err)
	}
	if ValidateCanvasShortcuts(map[string]string{"unknown": "F8"}) == nil {
		t.Fatal("unknown action accepted")
	}
}

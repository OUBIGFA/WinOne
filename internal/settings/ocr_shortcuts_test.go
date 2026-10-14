package settings

import "testing"

func TestNewShortcutDefaultsDoNotInvalidateExistingBindings(t *testing.T) {
	s := Settings{SchemaVersion: 1, Theme: "system", CaptureShortcut: "Shift+Alt+A", PinShortcut: "Ctrl+Shift+Q"}
	if err := Validate(s); err != nil {
		t.Fatal("new defaults broke an existing configuration", err)
	}
	if s.OCRBinding() != "Shift+C" || s.TranslateBinding() != "" || s.QuickAssistantBinding() != "" {
		t.Fatal("new defaults claimed an existing action's shortcut")
	}
	key := "Alt+Shift+A"
	s.OCRShortcut = &key
	if err := Validate(s); err == nil {
		t.Fatal("explicit duplicate shortcut was accepted")
	}
}

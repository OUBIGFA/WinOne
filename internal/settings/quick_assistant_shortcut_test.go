package settings

import "testing"

func TestAssistantShortcutDefaultsAndExplicitPreferences(t *testing.T) {
	s := Settings{SchemaVersion: 1, Theme: "system", CaptureShortcut: "Shift+A", PinShortcut: "Alt+Q"}
	if s.OCRBinding() != "Shift+C" || s.TranslateBinding() != "Alt+Shift+A" {
		t.Fatalf("unexpected capture defaults: %+v", s.ResolvedShortcuts())
	}
	if s.QuickAssistantBinding() != "Ctrl+Shift+Q" {
		t.Fatal("assistant shortcut missing")
	}
	old := "Ctrl+Shift+Q"
	s.TranslateShortcut = &old
	if s.TranslateBinding() != old || s.QuickAssistantBinding() != "" {
		t.Fatal("must preserve an existing binding without introducing a conflict")
	}
	disabled := ""
	s.QuickAssistantShortcut = &disabled
	resolved := s.ResolvedShortcuts()
	*resolved.QuickAssistantShortcut = "Alt+X"
	if s.QuickAssistantBinding() != "" {
		t.Fatal("resolved settings mutated user preferences")
	}
}

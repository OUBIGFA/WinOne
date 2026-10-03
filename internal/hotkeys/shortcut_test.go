package hotkeys

import "testing"

func TestShortcutValidation(t *testing.T) {
	for _, v := range []string{"", "Ctrl+Alt+S", "Win+Ctrl+T", "Shift+F24"} {
		if _, err := Parse(v); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
	}
	for _, v := range []string{"S", "Ctrl", "Ctrl+Ctrl+A", "Ctrl+A+B", "Ctrl+F12", "Alt+F25", "Ctrl+PrintScreen"} {
		if _, err := Parse(v); err == nil {
			t.Fatalf("accepted %s", v)
		}
	}
	if err := validate([]Binding{{"capture", "Ctrl+Alt+S"}, {"pin", "alt+ctrl+s"}}); err == nil {
		t.Fatal("duplicate keys accepted")
	}
}

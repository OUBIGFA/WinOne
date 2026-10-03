package hotkeys

import "testing"

func TestNumpadShortcutKeepsItsVirtualKey(t *testing.T) {
	top, err := Parse("Ctrl+1")
	if err != nil {
		t.Fatal(err)
	}
	pad, err := Parse("Ctrl+Num1")
	if err != nil {
		t.Fatal(err)
	}
	if pad.Key != 0x61 || pad.Key == top.Key || pad.Modifiers != top.Modifiers {
		t.Fatal(top, pad)
	}
}

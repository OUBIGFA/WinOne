package selection

import "testing"

func TestGestureRecognizesSelectionButNotClicksOrWindowChanges(t *testing.T) {
	g := Gesture{DragX: 4, DragY: 4, DoubleClickMilliseconds: 500}
	g.Pointer(true, 10, 10, 100, 1, false)
	if g.Pointer(false, 10, 10, 110, 1, false) {
		t.Fatal("single click is not a selection")
	}
	g.Pointer(true, 11, 10, 200, 1, false)
	if !g.Pointer(false, 11, 10, 210, 1, false) {
		t.Fatal("double click was missed")
	}
	g.Pointer(true, 10, 10, 900, 1, false)
	if !g.Pointer(false, 160, 80, 1000, 1, false) {
		t.Fatal("drag selection was missed")
	}
	g.Pointer(true, 10, 10, 1500, 1, false)
	if g.Pointer(false, 160, 80, 1600, 2, false) {
		t.Fatal("selection crossed source windows")
	}
	g.Pointer(true, 10, 10, 1700, 1, true)
	if !g.Pointer(false, 10, 10, 1710, 1, true) {
		t.Fatal("shift click was missed")
	}
}

func TestKeyboardSelectionDoesNotTreatOrdinaryTypingAsSelection(t *testing.T) {
	for _, key := range []uint32{0x25, 0x27, 0x26, 0x28, 0x24, 0x23, 0x21, 0x22} {
		if !SelectionKey(key, true, false, false) {
			t.Fatalf("shift-selection missed: %x", key)
		}
	}
	if !SelectionKey('A', false, true, false) {
		t.Fatal("select all was missed")
	}
	if SelectionKey('A', false, false, false) || SelectionKey('C', false, true, false) || SelectionKey(0x25, true, false, true) {
		t.Fatal("nonselection input accepted")
	}
}

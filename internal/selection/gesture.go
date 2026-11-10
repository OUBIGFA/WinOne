// Package selection implements WinOne's global selection module without owning
// a second result UI or provider configuration.
package selection

type Gesture struct {
	DragX, DragY            int32
	DoubleClickMilliseconds uint32
	down, previousClick     bool
	x, y, clickX, clickY    int32
	window, clickWindow     uintptr
	clickTime               uint32
}

func (g *Gesture) Pointer(down bool, x, y int32, stamp uint32, window uintptr, shift bool) bool {
	if down {
		g.down, g.x, g.y, g.window = true, x, y, window
		return false
	}
	if !g.down || window == 0 || window != g.window {
		g.down, g.previousClick = false, false
		return false
	}
	g.down = false
	distance := func(a, b int32) int64 {
		delta := int64(a) - int64(b)
		if delta < 0 {
			return -delta
		}
		return delta
	}
	drag := distance(x, g.x) > int64(g.DragX) || distance(y, g.y) > int64(g.DragY)
	double := g.previousClick && g.clickWindow == window && stamp-g.clickTime <= g.DoubleClickMilliseconds && distance(x, g.clickX) <= int64(g.DragX) && distance(y, g.clickY) <= int64(g.DragY)
	// Keep the click chain: the third click commonly expands a word to a
	// paragraph. Dropping it after a double click also dismisses its toolbar.
	g.previousClick, g.clickX, g.clickY, g.clickWindow, g.clickTime = !drag, x, y, window, stamp
	return drag || double || shift
}

func SelectionKey(key uint32, shift, control, alt bool) bool {
	if alt {
		return false
	}
	if control && key == 'A' {
		return true
	}
	return shift && key >= 0x21 && key <= 0x28
}

// Remember the chord at key-down. Users can release Shift/Ctrl before the
// selection key; sampling modifiers only at key-up loses that completed edit.
type KeyboardGesture struct{ selected [256]bool }

func (g *KeyboardGesture) Key(down bool, key uint32, shift, control, alt bool) bool {
	if key >= uint32(len(g.selected)) {
		return false
	}
	if down {
		g.selected[key] = SelectionKey(key, shift, control, alt)
		return false
	}
	selected := g.selected[key]
	g.selected[key] = false
	return selected
}

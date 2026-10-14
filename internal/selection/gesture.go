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
	g.previousClick, g.clickX, g.clickY, g.clickWindow, g.clickTime = !drag && !double, x, y, window, stamp
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

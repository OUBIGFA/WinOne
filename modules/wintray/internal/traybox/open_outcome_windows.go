//go:build windows

package traybox

// This describes observed window state, not a guessed application preference.
// Native collection must validate the process identity before using it.
type openWindowState struct {
	handle                                  uintptr
	pid                                     uint32
	visible, minimized, cloaked, foreground bool
}

func (w openWindowState) shown() bool { return w.visible && !w.minimized && !w.cloaked }

// observedOpen requires an observable transition in the target process.
// Existing visible windows and unrelated applications cannot teach a click mode.
func observedOpen(before, after []openWindowState, pid uint32) bool {
	if pid == 0 {
		return false
	}
	old := make(map[uintptr]openWindowState, len(before))
	for _, window := range before {
		if window.pid == pid && window.handle != 0 {
			old[window.handle] = window
		}
	}
	for _, window := range after {
		if window.pid != pid || window.handle == 0 || !window.shown() {
			continue
		}
		previous, exists := old[window.handle]
		if !exists || !previous.shown() || window.foreground && !previous.foreground {
			return true
		}
	}
	return false
}

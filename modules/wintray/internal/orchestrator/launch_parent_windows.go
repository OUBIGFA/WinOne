//go:build windows

package orchestrator

import (
	"errors"

	"golang.org/x/sys/windows"
	"winone/internal/desktopparent"
)

// openDesktopParent and processElevated are variables so tests can exercise
// the fallback decisions without depending on the user's Explorer.
var (
	openDesktopParent = desktopparent.Open
	processElevated   = func() bool { return windows.GetCurrentProcessToken().IsElevated() }
)

// launchUnderDesktop starts managed programs under the desktop shell, as
// Windows startup and the program's own startup task do. They receive the
// shell's token rather than an elevated WinOne's, and Task Manager no longer
// counts their memory in WinOne's group.
//
// Behavior stays unchanged where the desktop cannot be used: before Explorer
// has a desktop, for another account's desktop, and for a program whose
// manifest requires administrator rights (Windows refuses that launch from a
// standard token; an elevated WinOne keeps starting it elevated directly).
func launchUnderDesktop(start func(*desktopparent.Parent) (*startedProcess, error)) (*startedProcess, error) {
	parent, err := openDesktopParent()
	if err != nil {
		return start(nil)
	}
	process, err := start(parent)
	parent.Close()
	if errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) && processElevated() {
		return start(nil)
	}
	return process, err
}

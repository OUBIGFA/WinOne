//go:build windows

package traybox

import (
	"errors"
	"github.com/lxn/win"
	"winone/internal/processidentity"
)

// Check the pinned process both before and after enumeration. Windows may
// reuse its numeric PID while a snapshot is in progress.
func snapshotOpenTarget(process *processidentity.Process) ([]openWindowState, error) {
	if process == nil {
		return nil, errors.New("missing pinned process")
	}
	gone, err := process.Exited()
	if err != nil {
		return nil, err
	}
	if gone {
		return nil, ErrIconGone
	}
	states, err := snapshotOpenWindows(process.PID)
	if err != nil {
		return nil, err
	}
	gone, err = process.Exited()
	if err != nil {
		return nil, err
	}
	if gone {
		return nil, ErrIconGone
	}
	return states, nil
}

// Keep the verified process handle until observation completes. Recheck the
// icon owner after opening it so a stale HWND cannot select a different PID.
func openIconTarget(icon Icon) (*processidentity.Process, error) {
	owner := win.HWND(icon.owner)
	var pid uint32
	if owner == 0 || win.GetWindowThreadProcessId(owner, &pid) == 0 || pid == 0 {
		return nil, ErrIconGone
	}
	process, err := processidentity.Open(pid, icon.ExePath)
	if err != nil {
		return nil, err
	}
	var current uint32
	if win.GetWindowThreadProcessId(owner, &current) == 0 || current != pid {
		process.Close()
		return nil, ErrIconGone
	}
	return process, nil
}

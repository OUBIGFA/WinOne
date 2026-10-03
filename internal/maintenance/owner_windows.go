//go:build windows

package maintenance

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

// HoldUntilOwnerExit acknowledges successful preparation through an event
// created by the installer, then keeps the caller's leases alive until that
// exact process exits. An open process handle avoids PID-reuse ambiguity.
// The caller must keep this process outside the installation being removed.
func HoldUntilOwnerExit(pid uint32, readyName string, prepare func() error) error {
	if pid == 0 || pid == uint32(os.Getpid()) || readyName == "" || prepare == nil {
		return errors.New("invalid installer owner")
	}
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(owner)
	key, err := windows.UTF16PtrFromString(readyName)
	if err != nil {
		return err
	}
	ready, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, key)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ready)
	alive := func() error {
		state, err := windows.WaitForSingleObject(owner, 0)
		if err != nil {
			return err
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("installer has exited")
		}
		return nil
	}
	if err = alive(); err != nil {
		return err
	}
	if err = prepare(); err != nil {
		return err
	}
	if err = alive(); err != nil {
		return err
	}
	if err = windows.SetEvent(ready); err != nil {
		return err
	}
	state, err := windows.WaitForSingleObject(owner, windows.INFINITE)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("unexpected installer wait result: %d", state)
	}
	return nil
}

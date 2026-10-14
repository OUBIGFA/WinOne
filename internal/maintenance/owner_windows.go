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
// exact process exits or its completion-release event is signalled. The optional
// event defaults to readyName + "-release"; absent events keep legacy behavior.
// An open process handle avoids PID-reuse ambiguity.
// The caller must keep this process outside the installation being removed.
func HoldUntilOwnerExit(pid uint32, readyName string, prepare func() error, releaseName ...string) error {
	if pid == 0 || pid == uint32(os.Getpid()) || readyName == "" || prepare == nil || len(releaseName) > 1 {
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
	// A successful finish page may release the guard before launching WinOne.
	// It must still wait for this helper to exit, so all caller leases are gone.
	var release windows.Handle
	name := readyName + "-release"
	explicit := len(releaseName) == 1 && releaseName[0] != ""
	if explicit {
		name = releaseName[0]
	}
	{
		if name == readyName {
			return errors.New("release and readiness events must differ")
		}
		key, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		release, err = windows.OpenEvent(windows.SYNCHRONIZE, false, key)
		if err != nil && (explicit || !errors.Is(err, windows.ERROR_FILE_NOT_FOUND)) {
			return err
		}
		if release != 0 {
			defer windows.CloseHandle(release)
		}
		if release != 0 {
			if state, err := windows.WaitForSingleObject(release, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
				return errors.New("installer release event is already signalled")
			}
		}
	}
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
	if release != 0 {
		state, err := windows.WaitForMultipleObjects([]windows.Handle{owner, release}, false, windows.INFINITE)
		if err != nil {
			return err
		}
		if state != windows.WAIT_OBJECT_0 && state != windows.WAIT_OBJECT_0+1 {
			return fmt.Errorf("unexpected installer release result: %d", state)
		}
		return nil
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

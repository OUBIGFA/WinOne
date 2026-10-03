//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"winone/internal/processidentity"
	"winone/internal/windowinstance"
)

type foregroundPermissionPort struct {
	find   func(string) (uint32, error)
	verify func(uint32, string) (func(), error)
	allow  func(uint32) error
}

// The tray-launched process may activate windows, but the already-running
// process does not inherit that right merely by receiving Wails' WM_COPYDATA.
func transferForegroundPermission(instance, executable string, port foregroundPermissionPort) error {
	pid, err := port.find(instance)
	if err != nil || pid == 0 {
		return err
	}
	release, err := port.verify(pid, executable)
	if err != nil {
		return err
	}
	defer release()
	return port.allow(pid)
}

func grantExistingWindowForeground(instance, executable string) error {
	return transferForegroundPermission(instance, executable, foregroundPermissionPort{
		find: windowinstance.FindPID,
		verify: func(pid uint32, path string) (func(), error) {
			process, err := processidentity.Open(pid, path)
			if err != nil {
				return nil, err
			}
			return process.Close, nil
		},
		allow: func(pid uint32) error {
			if ok, _, err := desktopUser32.NewProc("AllowSetForegroundWindow").Call(uintptr(pid)); ok == 0 {
				if err == windows.ERROR_SUCCESS {
					return errors.New("Windows did not grant foreground permission")
				}
				return err
			}
			return nil
		},
	})
}

//go:build windows

package main

import (
	"context"
	"errors"
	"time"
	"winone/internal/processidentity"
)

func waitRestartExit(ctx context.Context, ownerExited, helperExited func() (bool, error), acknowledged func() bool, interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		gone, err := helperExited()
		if err != nil {
			return err
		}
		if gone {
			return errors.New("管理员辅助进程已退出，请重试")
		}
		closed, err := ownerExited()
		if err != nil {
			return err
		}
		if closed && acknowledged() {
			gone, err = helperExited()
			if err != nil {
				return err
			}
			if gone {
				return errors.New("管理员辅助进程已退出，请重试")
			}
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Keep native handles until the handoff finishes; PID reuse cannot substitute
// another window or helper. No process is forcibly terminated.
func openRestartExitWait(ownerPID, helperPID uint32, executable string, timeout time.Duration) (func(func() bool) error, func(), error) {
	helper, err := processidentity.Open(helperPID, executable)
	if err != nil {
		return nil, nil, err
	}
	if !helper.Elevated {
		helper.Close()
		return nil, nil, errors.New("替换进程未获得管理员权限")
	}
	owner, err := processidentity.Open(ownerPID, executable)
	if err != nil && !errors.Is(err, processidentity.ErrExited) {
		helper.Close()
		return nil, nil, err
	}
	closeHandles := func() {
		if owner != nil {
			owner.Close()
		}
		helper.Close()
	}
	wait := func(acknowledged func() bool) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		ownerExited := func() (bool, error) {
			if owner == nil {
				return true, nil
			}
			return owner.Exited()
		}
		return waitRestartExit(ctx, ownerExited, helper.Exited, acknowledged, 100*time.Millisecond)
	}
	return wait, closeHandles, nil
}

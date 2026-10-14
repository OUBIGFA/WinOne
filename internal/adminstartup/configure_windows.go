//go:build windows

package adminstartup

import (
	"context"
	"errors"
	"time"
)

// ConfigureAuthorization reuses the fixed launcher and verifies the resulting
// task state after consent. It neither stops the Host nor saves preferences.
func ConfigureAuthorization(enable, elevated bool, exe, sid string, task AuthorizationTask, launch func(string, []string) (uint32, func() error, error)) error {
	ready, readErr := task.Configured()
	if readErr == nil && ready == enable {
		return nil
	}
	if !enable && readErr == nil && ready {
		if revoker, ok := task.(AuthorizationRevoker); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return RevokeAuthorized(ctx, revoker, elevated)
		}
	}
	if elevated {
		if err := ApplyAuthorization(enable, true, sid, sid, task); err != nil {
			return err
		}
	} else {
		operation := "--authorize-administrator"
		if !enable {
			operation = "--revoke-administrator"
		}
		_, wait, err := launch(exe, []string{operation, sid})
		if err != nil {
			return err
		}
		if wait == nil {
			return errors.New("无法确认管理员授权结果")
		}
		if err = wait(); err != nil {
			return err
		}
	}
	ready, err := task.Configured()
	if err != nil {
		return err
	}
	if ready != enable {
		return errors.New("管理员授权状态未能更新")
	}
	return nil
}

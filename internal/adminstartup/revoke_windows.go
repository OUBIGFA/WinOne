//go:build windows

package adminstartup

import (
	"context"
	"errors"
	"time"
	"winone/internal/logintask"
)

type AuthorizationRevoker interface {
	Launcher
	Remove() error
}

// The authorized task may revoke itself. Ordinary callers need no second UAC
// prompt, and completion is established by confirmed absence, not task acceptance.
func RevokeAuthorized(ctx context.Context, task AuthorizationRevoker, elevated bool) error {
	ready, err := task.Configured()
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	if elevated {
		return task.Remove()
	}
	_, runErr := task.Run(logintask.RevokeAuthorization)
	for {
		ready, err = task.Configured()
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
		if runErr != nil {
			return runErr
		}
		select {
		case <-ctx.Done():
			return errors.New("未能确认管理员授权已撤销")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

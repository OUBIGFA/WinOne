//go:build windows

package main

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"os"
	"time"
	"winone/internal/adminstartup"
	"winone/internal/host"
	"winone/internal/logintask"
)

type administratorRestartPorts struct {
	guard  func() error
	call   func(string, any) (administratorHandoffSnapshot, error)
	launch func() error
	close  func(string) error
}

func performAdministratorRestart(ctx context.Context, p administratorRestartPorts) error {
	return adminstartup.Restart(ctx, adminstartup.RestartPorts{Guard: p.guard, Call: p.call, Launch: p.launch, Close: p.close})
}

func (a *App) checkAdministratorRestart() error {
	a.ctxMu.RLock()
	blocked := a.settingsDirty || a.captureEditing
	ctx := a.ctx
	a.ctxMu.RUnlock()
	if ctx == nil {
		return errors.New("设置窗口未就绪")
	}
	if blocked || a.captureLaunches.Peek() != nil {
		return errors.New("请先完成当前编辑并保存设置")
	}
	return nil
}

// The authorization mutex serializes permission changes across the whole flow.
// A true result means close was requested, not that the replacement is ready.
func (a *App) RestartAsAdministrator() (bool, error) {
	a.authorizationMu.Lock()
	defer a.authorizationMu.Unlock()
	if err := a.checkAdministratorRestart(); err != nil {
		return false, err
	}
	var snapshot host.Snapshot
	if err := a.request("snapshot", nil, &snapshot); err != nil {
		return false, err
	}
	if snapshot.Administrator && windows.GetCurrentProcessToken().IsElevated() {
		return false, nil
	}
	if err := a.configureAdministratorAuthorization(true); err != nil {
		return false, err
	}
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	launcher, err := logintask.NewAdministratorLauncher(executable)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	err = performAdministratorRestart(ctx, administratorRestartPorts{
		guard: a.checkAdministratorRestart,
		call: func(method string, params any) (administratorHandoffSnapshot, error) {
			var result administratorHandoffSnapshot
			err := a.request("application."+method, params, &result)
			return result, err
		},
		launch: func() error { _, err := launcher.Run(logintask.InteractiveLaunch); return err },
		close: func(token string) error {
			if err := a.checkAdministratorRestart(); err != nil {
				return err
			}
			a.ctxMu.Lock()
			a.settingsCloseApproved = true
			ctx := a.ctx
			a.ctxMu.Unlock()
			runtime.Quit(ctx)
			// Keep the UI operation pending until Wails actually closes. If its
			// close guard intervenes, the Host's rollback ends the pending state.
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(100 * time.Millisecond):
				}
				var state administratorHandoffSnapshot
				if err := a.request("application.restart.status", map[string]string{"token": token}, &state); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
				if state.Token != token {
					return errors.New("重启准备已变化")
				}
				if state.State == "cancelled" || state.State == "failed" {
					if state.Error == "" {
						return errors.New("重启未完成，请重试")
					}
					return errors.New(state.Error)
				}
			}
		},
	})
	return err == nil, err
}

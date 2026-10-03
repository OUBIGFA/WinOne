//go:build windows

package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/adminstartup"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/internal/transport"
)

type administratorAuthorizationTask interface {
	Configured() (bool, error)
	Register() error
	Remove() error
}

func (a *App) SetAdministratorPreference(enabled bool) (settings.Settings, error) {
	a.authorizationMu.Lock()
	defer a.authorizationMu.Unlock()
	var snapshot host.Snapshot
	if err := a.request("snapshot", nil, &snapshot); err != nil {
		return settings.Settings{}, err
	}
	if err := a.configureAdministratorAuthorization(enabled); err != nil {
		return snapshot.Settings, err
	}
	// UAC can remain open while other settings change. Read the latest revision
	// after consent rather than overwriting unrelated preferences from before it.
	if err := a.request("snapshot", nil, &snapshot); err != nil {
		return settings.Settings{}, err
	}
	if snapshot.Settings.Startup == nil {
		snapshot.Settings.Startup = &settings.StartupPreferences{StartMinimized: true}
	}
	snapshot.Settings.Startup.PreferAdministrator = enabled
	return a.SaveSettings(snapshot.Settings)
}

// The consent helper accepts no path, script or forwarded application flags.
// Another administrator's credentials cannot grant the originating user's task.
func administratorAuthorizationRequest(args []string) (enable bool, sid string, handled bool, err error) {
	if len(args) == 0 || (args[0] != "--authorize-administrator" && args[0] != "--revoke-administrator") {
		return false, "", false, nil
	}
	if len(args) != 2 {
		return false, "", true, errors.New("管理员授权参数无效")
	}
	if _, err = windows.StringToSid(args[1]); err != nil {
		return false, "", true, errors.New("管理员授权账户无效")
	}
	return args[0] == "--authorize-administrator", args[1], true, nil
}
func applyAdministratorAuthorization(enable, elevated bool, expectedSID, currentSID string, task administratorAuthorizationTask) error {
	if !elevated {
		return errors.New("需要先确认管理员授权")
	}
	if expectedSID == "" || expectedSID != currentSID {
		return errors.New("请使用当前 Windows 账户确认授权")
	}
	if enable {
		return task.Register()
	}
	return task.Remove()
}
func runAdministratorAuthorizationHelper(args []string, executable string) (bool, error) {
	enable, expected, handled, err := administratorAuthorizationRequest(args)
	if !handled || err != nil {
		return handled, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return true, err
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return true, errors.New("需要先确认管理员授权")
	}
	if user.User.Sid.String() != expected {
		return true, errors.New("请使用当前 Windows 账户确认授权")
	}
	if enable {
		lease, err := maintenance.AcquireRuntimeForInstallation(filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), filepath.Dir(executable))
		if err != nil {
			return true, err
		}
		defer lease.Close()
	}
	launcher, err := logintask.NewAdministratorLauncher(executable)
	if err != nil {
		return true, err
	}
	return true, applyAdministratorAuthorization(enable, windows.GetCurrentProcessToken().IsElevated(), expected, user.User.Sid.String(), launcher)
}

// Authorizing a launcher never stops the current Host or changes user settings.
// The caller must separately complete restart handoff and persist its preference.
func configureAdministratorAuthorization(enable, elevated bool, exe, sid string, task administratorAuthorizationTask, launch func(string, []string) (uint32, func() error, error)) error {
	ready, readErr := task.Configured()
	if readErr == nil && ready == enable {
		return nil
	}
	if !enable && readErr == nil && ready {
		if revoker, ok := task.(adminstartup.AuthorizationRevoker); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return adminstartup.RevokeAuthorized(ctx, revoker, elevated)
		}
	}
	if elevated {
		if err := applyAdministratorAuthorization(enable, true, sid, sid, task); err != nil {
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

func (a *App) ConfigureAdministratorAuthorization(enabled bool) error {
	a.authorizationMu.Lock()
	defer a.authorizationMu.Unlock()
	return a.configureAdministratorAuthorization(enabled)
}
func (a *App) configureAdministratorAuthorization(enabled bool) error {
	if !strings.EqualFold(filepath.Clean(a.dataDirectory), filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")) {
		return errors.New("独立测试配置不允许更改管理员授权")
	}
	expectedPipe, err := transport.PipeName()
	if err != nil {
		return err
	}
	if a.pipe != expectedPipe {
		return errors.New("独立测试配置不允许更改管理员授权")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	task, err := logintask.NewAdministratorLauncher(executable)
	if err != nil {
		return err
	}
	return configureAdministratorAuthorization(enabled, windows.GetCurrentProcessToken().IsElevated(), executable, user.User.Sid.String(), task, modules.LaunchElevated)
}

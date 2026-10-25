//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/distribution"
	"winone/internal/logintask"
	"winone/internal/settings"
)

type productStartupTask interface {
	Sync(string, string) error
	Remove() error
}

// Reuse the installer's read-only ownership check, including resolved directory
// paths. A copied executable must not adopt the installed app's login entry just
// because it opened the same default profile. Portable installs remain supported
// when no installer registration exists.
func productStartupOwner(executable string) error {
	_, err := distribution.CaptureRegistration(filepath.Dir(executable), `Software\Microsoft\Windows\CurrentVersion\Uninstall\WinOne`)
	if err != nil {
		return fmt.Errorf("当前副本不能修改已安装 WinOne 的登录启动项，请从安装目录打开 WinOne：%w", err)
	}
	return nil
}

// Only the normal user profile owns the real login entry. Test profiles never
// create, replace, or delete another installation's scheduled task.
func syncProductStartup(directory, executable string, value settings.Settings) error {
	return syncProductStartupWith(directory, executable, value, productStartupOwner, func() (productStartupTask, error) { return logintask.NewProductLogonTask() })
}

// The OS boundaries are injected so regression tests cannot touch real tasks.
func syncProductStartupWith(directory, executable string, value settings.Settings, checkOwner func(string) error, openTask func() (productStartupTask, error)) error {
	if value.Startup == nil {
		return nil
	}
	normal := filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")
	if !strings.EqualFold(filepath.Clean(directory), filepath.Clean(normal)) {
		if value.Startup.RunAtLogon {
			return errors.New("独立测试配置不允许修改系统自启")
		}
		return nil
	}
	// Profile identity alone is insufficient: test/candidate copies can also be
	// launched without --data-dir and inherit the installed user's preferences.
	if err := checkOwner(executable); err != nil {
		return err
	}
	task, err := openTask()
	if err != nil {
		return err
	}
	if value.Startup.RunAtLogon {
		return task.Sync(executable, "--autorun")
	}
	return task.Remove()
}

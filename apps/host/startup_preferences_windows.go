//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/logintask"
	"winone/internal/settings"
)

// Only the normal user profile owns the real login entry. Test profiles never
// create, replace, or delete another installation's scheduled task.
func syncProductStartup(directory, executable string, value settings.Settings) error {
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
	task, err := logintask.NewProductLogonTask()
	if err != nil {
		return err
	}
	if value.Startup.RunAtLogon {
		return task.Sync(executable, "--autorun")
	}
	return task.Remove()
}

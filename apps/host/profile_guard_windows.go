//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/distribution"
)

// A copy outside the installed directory must never silently open the real
// user profile: catalog migrations there would bump schema versions the
// installed build cannot read back. The installer's read-only ownership check
// decides which copy owns the registration; portable use stays possible when
// no installer registration exists.
func guardDefaultProfile(data string, dataDirExplicit bool) error {
	normal := filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")
	if !strings.EqualFold(filepath.Clean(data), filepath.Clean(normal)) || dataDirExplicit {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := distribution.CaptureRegistration(filepath.Dir(executable), `Software\Microsoft\Windows\CurrentVersion\Uninstall\WinOne`); err != nil {
		return errors.New("当前副本不属于已安装的 WinOne，为保护真实配置已停止；请从安装目录打开，或测试时用 --data-dir 指定独立目录")
	}
	return nil
}

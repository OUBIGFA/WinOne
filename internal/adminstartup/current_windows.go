//go:build windows

package adminstartup

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/logintask"
	"winone/internal/modules"
	"winone/internal/transport"
)

// ValidateDefaultProfile prevents isolated/test profiles from changing the
// current account's fixed administrator launcher.
func ValidateDefaultProfile(pipe, directory string) error {
	if !strings.EqualFold(filepath.Clean(directory), filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")) {
		return errors.New("独立测试配置不允许更改管理员授权")
	}
	expected, err := transport.PipeName()
	if err != nil {
		return err
	}
	if pipe != expected {
		return errors.New("独立测试配置不允许更改管理员授权")
	}
	return nil
}

// ConfigureCurrent keeps both desktop entrypoints on the same consent path.
func ConfigureCurrent(enabled bool, pipe, directory, executable string) error {
	if err := ValidateDefaultProfile(pipe, directory); err != nil {
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
	return ConfigureAuthorization(enabled, windows.GetCurrentProcessToken().IsElevated(), executable, user.User.Sid.String(), task, modules.LaunchElevated)
}

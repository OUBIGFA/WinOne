//go:build windows

package adminstartup

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"winone/internal/logintask"
	"winone/internal/maintenance"
)

// AuthorizationTask is the existing fixed Windows scheduled-task capability.
type AuthorizationTask interface {
	Configured() (bool, error)
	Register() error
	Remove() error
}

// AuthorizationRequest accepts only a fixed operation and the requesting SID.
func AuthorizationRequest(args []string) (enable bool, sid string, handled bool, err error) {
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

// ApplyAuthorization requires both elevation and the original Windows account.
func ApplyAuthorization(enable, elevated bool, expectedSID, currentSID string, task AuthorizationTask) error {
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

// RunAuthorizationHelper serves both desktop entrypoints without UI dependencies.
func RunAuthorizationHelper(args []string, executable string) (bool, error) {
	enable, expected, handled, err := AuthorizationRequest(args)
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
	return true, ApplyAuthorization(enable, windows.GetCurrentProcessToken().IsElevated(), expected, user.User.Sid.String(), launcher)
}

//go:build windows

package logintask

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

type LaunchMode string

const (
	InteractiveLaunch   LaunchMode = "interactive"
	LoginLaunch         LaunchMode = "login"
	RevokeAuthorization LaunchMode = "revoke"
)

// AdministratorLauncher binds one current-user task to this installation's
// fixed WinOne entry point. Run never accepts an executable or command line.
type AdministratorLauncher struct {
	task       *LogonTask
	executable string
	invoke     func(string, LaunchMode, string, string) (string, error)
}

func NewAdministratorLauncher(executable string) (*AdministratorLauncher, error) {
	if !filepath.IsAbs(executable) || !strings.EqualFold(filepath.Base(executable), "WinOne.exe") {
		return nil, errors.New("管理员启动入口必须为 WinOne.exe")
	}
	task, err := NewProductLogonTask()
	if err != nil {
		return nil, err
	}
	task.name = "WinOne-Administrator-" + task.userSID
	task.elevated = true
	task.onDemand = true
	return &AdministratorLauncher{task: task, executable: filepath.Clean(executable), invoke: runAdministratorTask}, nil
}

func (a *AdministratorLauncher) checkExecutable() error {
	info, err := os.Stat(a.executable)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("WinOne 启动文件不可用")
	}
	return nil
}

// Register must run inside the helper which the user has approved through UAC.
// The task is not considered configured until the registered definition matches.
func (a *AdministratorLauncher) Register() error {
	if err := a.checkExecutable(); err != nil {
		return err
	}
	if err := a.task.Sync(a.executable, administratorLaunchArguments); err != nil {
		return err
	}
	ready, err := a.Configured()
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("管理员启动任务尚未注册")
	}
	return nil
}
func (a *AdministratorLauncher) Configured() (bool, error) {
	err := withScheduledTask(a.task.name, func(task *ole.IDispatch) error { return checkAdministratorTask(task, a.task.userSID, a.executable) })
	if taskNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
func (a *AdministratorLauncher) Remove() error { return a.task.Remove() }

// The returned instance ID acknowledges scheduler acceptance, not process
// readiness. The caller must still verify the new Host before finishing handoff.
func (a *AdministratorLauncher) Run(mode LaunchMode) (string, error) {
	if mode != InteractiveLaunch && mode != LoginLaunch && mode != RevokeAuthorization {
		return "", errors.New("不支持的管理员启动方式")
	}
	if err := a.checkExecutable(); err != nil {
		return "", err
	}
	return a.invoke(a.task.name, mode, a.task.userSID, a.executable)
}
func checkAdministratorTask(task *ole.IDispatch, sid, executable string) error {
	definition, err := oleutil.GetProperty(task, "Xml")
	if definition != nil {
		defer definition.Clear()
	}
	if err != nil {
		return err
	}
	if definition == nil || definition.VT != ole.VT_BSTR {
		return errors.New("无法读取管理员启动任务")
	}
	return validateAdministratorDefinition([]byte(definition.ToString()), sid, executable)
}
func runAdministratorTask(name string, mode LaunchMode, sid, executable string) (string, error) {
	var instance string
	err := withScheduledTask(name, func(task *ole.IDispatch) error {
		if err := checkAdministratorTask(task, sid, executable); err != nil {
			return err
		}
		running, err := oleutil.CallMethod(task, "Run", string(mode))
		if running != nil {
			defer running.Clear()
		}
		if err != nil {
			return err
		}
		if running == nil || running.VT != ole.VT_DISPATCH || running.ToIDispatch() == nil {
			return errors.New("管理员启动任务未返回运行实例")
		}
		id, err := oleutil.GetProperty(running.ToIDispatch(), "InstanceGuid")
		if id != nil {
			defer id.Clear()
		}
		if err != nil {
			return err
		}
		if id == nil || id.VT != ole.VT_BSTR {
			return errors.New("管理员启动实例标识无效")
		}
		instance = id.ToString()
		if _, err := windows.GUIDFromString(instance); err != nil {
			return errors.New("管理员启动实例标识无效")
		}
		return nil
	})
	return instance, err
}

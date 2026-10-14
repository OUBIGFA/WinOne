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
	"winone/internal/directorylinks"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/modules"
	"winone/internal/transport"
)

// Preparation is deliberately an offline operation. The caller holds the Host
// pipe reservation; no normal configuration load or login activation occurs.
// Installers must still prevent relaunch throughout their later file operations.
func prepareUninstall(data, pipe string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return prepareUninstallAt(filepath.Dir(executable), data, pipe)
}

func prepareUninstallAt(installation, data, pipe string) error {
	if err := maintenance.RequireStoppedAt(installation); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	worker := modules.NewLazy(ctx, "wintray", "WinTray", filepath.Join(installation, "WinOne.WinTray.exe"), pipe+"-wintray", filepath.Join(data, "modules", "wintray"))
	if err := releaseWinTray(ctx, worker, "uninstall.prepare"); err != nil {
		return err
	}
	if strings.EqualFold(filepath.Clean(data), filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")) {
		task, err := logintask.NewAdministratorLauncher(filepath.Join(installation, "WinOne.exe"))
		if err != nil {
			return err
		}
		if err = adminstartup.RevokeAuthorized(ctx, task, windows.GetCurrentProcessToken().IsElevated()); err != nil && !errors.Is(err, logintask.ErrAdministratorOtherInstallation) {
			return err
		}
	}
	if err := directorylinks.RemoveExplorerMenuOwnedBy(filepath.Join(installation, "WinOne.exe")); err != nil {
		return err
	}
	return maintenance.RequireStoppedAt(installation)
}

// The installer launches a temporary copy of Host so its persistent lease does
// not pin an executable inside the directory it needs to replace or remove.
func guardInstallation(installation, data, pipe string, owner uint32, ready string, uninstall bool, release string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(installation)
	if err != nil {
		return err
	}
	helper, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.VolumeName(resolved), filepath.VolumeName(helper)) {
		relative, err := filepath.Rel(resolved, helper)
		if err != nil {
			return err
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("维护程序必须从安装目录外运行")
		}
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(data, installation)
	if err != nil {
		return err
	}
	defer lease.Close()
	listener, err := transport.Listen(pipe)
	if err != nil {
		return err
	}
	defer listener.Close()
	return maintenance.HoldUntilOwnerExit(owner, ready, func() error {
		if uninstall {
			return prepareUninstallAt(installation, data, pipe)
		}
		return maintenance.RequireStoppedAt(installation)
	}, release)
}

func isWinOneProcess(name string) bool { return maintenance.IsProductProcessName(name) }

//go:build windows

// Package installguard shares the offline installer protocol with legacy Host
// commands. It does not start Host services, discover accounts or enable login.
package installguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"winone/internal/adminstartup"
	"winone/internal/directorylinks"
	"winone/internal/interconnect"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/modules"
	"winone/internal/transport"
)

// Request identifies the installation, profile and exact installer process.
// The helper must run outside Installation so its image cannot pin the payload.
type Request struct {
	Installation     string
	Profile          string
	Pipe             string
	OwnerPID         uint32
	ReadyEvent       string
	ReleaseEvent     string
	PrepareUninstall bool
}

// PrepareUninstall preserves the existing offline resource-restoration path.
// The caller already owns the maintenance leases and Host pipe reservation.
// No normal configuration load or login activation occurs here.
func PrepareUninstall(installation, profile, pipe string) error {
	if err := maintenance.RequireStoppedAt(installation); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	worker := modules.NewLazy(ctx, "wintray", "WinTray", filepath.Join(installation, "WinOne.WinTray.exe"), pipe+"-wintray", filepath.Join(profile, "modules", "wintray"))
	if err := modules.ReleaseWinTray(ctx, worker, true); err != nil {
		return err
	}
	if strings.EqualFold(filepath.Clean(profile), filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")) {
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
	if err := interconnect.RemoveExplorerMenuOwnedBy(filepath.Join(installation, "WinOne.exe")); err != nil {
		return err
	}
	return maintenance.RequireStoppedAt(installation)
}

// Hold keeps both leases and the pipe until preparation succeeds and the owner
// exits or explicitly releases the guard. Both the dedicated helper and older
// Host CLI use this exact protocol; readiness never substitutes for cleanup.
func Hold(request Request) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(request.Installation)
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
	lease, err := maintenance.AcquireMaintenanceForInstallation(request.Profile, request.Installation)
	if err != nil {
		return err
	}
	defer lease.Close()
	listener, err := transport.Listen(request.Pipe)
	if err != nil {
		return err
	}
	defer listener.Close()
	return maintenance.HoldUntilOwnerExit(request.OwnerPID, request.ReadyEvent, func() error {
		if request.PrepareUninstall {
			return PrepareUninstall(request.Installation, request.Profile, request.Pipe)
		}
		return maintenance.RequireStoppedAt(request.Installation)
	}, request.ReleaseEvent)
}

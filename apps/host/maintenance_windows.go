//go:build windows

package main

import (
	"os"
	"path/filepath"
	"winone/internal/installguard"
	"winone/internal/maintenance"
)

// Keep the public Host commands compatible with previously installed packages.
// New installers use the dedicated helper, without Host's runtime entry points.
func prepareUninstall(data, pipe string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return installguard.PrepareUninstall(filepath.Dir(executable), data, pipe)
}

func guardInstallation(installation, data, pipe string, owner uint32, ready string, uninstall bool, release string) error {
	return installguard.Hold(installguard.Request{
		Installation:     installation,
		Profile:          data,
		Pipe:             pipe,
		OwnerPID:         owner,
		ReadyEvent:       ready,
		ReleaseEvent:     release,
		PrepareUninstall: uninstall,
	})
}

func isWinOneProcess(name string) bool { return maintenance.IsProductProcessName(name) }

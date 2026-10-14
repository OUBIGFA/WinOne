//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"winone/internal/diagnostics"

	"golang.org/x/sys/windows"
)

type ModuleLogEntry = diagnostics.ModuleLogEntry
type ModuleLogs = diagnostics.ModuleLogs

// ReadModuleLogs stays available even when the Host is not running. Both UI
// implementations share the same file allowlist, bounds and root containment.
func (a *App) ReadModuleLogs(id string) (ModuleLogs, error) {
	return diagnostics.Read(a.dataDirectory, id)
}

// OpenModuleLogFolder opens only a validated, existing module directory.
func (a *App) OpenModuleLogFolder(id string) error {
	directory, err := diagnostics.Folder(a.dataDirectory, id)
	if err != nil {
		return err
	}
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return err
	}
	command := exec.Command(filepath.Join(windowsDir, "explorer.exe"), directory)
	if err = command.Start(); err != nil {
		return err
	}
	go func() {
		if err := command.Wait(); err != nil {
			fmt.Fprintln(os.Stderr, "log folder:", err)
		}
	}()
	return nil
}

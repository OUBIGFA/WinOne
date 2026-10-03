//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"winone/internal/capturehandoff"
)

func launchDesktop(executable, pipe, directory, draftID string) error {
	cmd, err := startDesktop(executable, pipe, directory, draftID)
	if err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintln(os.Stderr, "WinOne window:", err)
		}
	}()
	return nil
}

func startDesktop(executable, pipe, directory, draftID string) (*exec.Cmd, error) {
	args := []string{"--pipe", pipe, "--data-dir", directory}
	if draftID == "" {
		args = append(args, "--open-settings")
	}
	if draftID != "" {
		if err := capturehandoff.ValidateDraftID(draftID); err != nil {
			return nil, err
		}
		args = append(args, "--capture-draft", draftID)
		// Inherit the Host token, including elevation. Do not redirect this
		// tracked screenshot process through a separate startup task.
		args = append(args, "--standard-session")
	}
	cmd := exec.Command(executable, args...)
	// Suppress a console; Wails controls whether the actual app window is shown.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

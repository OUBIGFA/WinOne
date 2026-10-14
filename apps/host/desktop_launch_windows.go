//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows"
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
	// A tray click belongs to Host; grant its newly started desktop process
	// permission to activate itself (and an already-open singleton window).
	// https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-allowsetforegroundwindow
	windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(uintptr(cmd.Process.Pid))
	return cmd, nil
}

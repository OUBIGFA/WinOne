//go:build windows

package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startConsoleProcess(command *exec.Cmd, hidden bool) (*startedProcess, error) {
	application, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return nil, err
	}
	line := windows.ComposeCommandLine(command.Args)
	if command.SysProcAttr != nil && command.SysProcAttr.CmdLine != "" {
		line = command.SysProcAttr.CmdLine
	}
	arguments, err := windows.UTF16PtrFromString(line)
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if command.Dir != "" {
		directory, err = windows.UTF16PtrFromString(command.Dir)
		if err != nil {
			return nil, err
		}
	}
	environment := command.Environ()
	for _, entry := range environment {
		if strings.ContainsRune(entry, 0) {
			return nil, fmt.Errorf("invalid console environment")
		}
	}
	sort.SliceStable(environment, func(i, j int) bool { return strings.ToUpper(environment[i]) < strings.ToUpper(environment[j]) })
	block := utf16.Encode([]rune(strings.Join(environment, "\x00") + "\x00\x00"))
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_SHOWNORMAL}
	if hidden {
		startup.ShowWindow = windows.SW_HIDE
	}
	var info windows.ProcessInformation
	// Omit STARTF_USESTDHANDLES and handle inheritance: Windows creates stdin,
	// stdout and stderr for this new console. Suspend only until ownership is
	// acquired, so an initialization failure cannot orphan a running application.
	if err = windows.CreateProcess(application, arguments, nil, nil, false, windows.CREATE_NEW_CONSOLE|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_SUSPENDED, &block[0], directory, &startup, &info); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(info.Thread)
	defer windows.CloseHandle(info.Process)
	rollback := func(cause error) error {
		terminateErr := windows.TerminateProcess(info.Process, 1)
		state, waitErr := windows.WaitForSingleObject(info.Process, 5000)
		if waitErr != nil || state != windows.WAIT_OBJECT_0 {
			return errors.Join(cause, terminateErr, waitErr, fmt.Errorf("failed console process %d was not released", info.ProcessId))
		}
		return cause
	}
	process, err := os.FindProcess(int(info.ProcessId))
	if err != nil {
		return nil, rollback(err)
	}
	if _, err = windows.ResumeThread(info.Thread); err != nil {
		err = rollback(err)
		_ = process.Release()
		return nil, err
	}
	return &startedProcess{Process: process, wait: func() error {
		state, err := process.Wait()
		if err != nil {
			return err
		}
		if !state.Success() {
			return &exec.ExitError{ProcessState: state}
		}
		return nil
	}}, nil
}

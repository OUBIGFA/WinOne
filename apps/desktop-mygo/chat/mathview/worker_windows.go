package mathview

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startBundledWorker starts the math worker beside this program, in a job that
// ends it with the window even if the window crashes.
func startBundledWorker() (*Worker, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return StartWorker(filepath.Join(filepath.Dir(executable), "shared", "ai", "runtime", "bun.exe"), filepath.Join(filepath.Dir(executable), "shared", "chat", "math.js"))
}

// StartWorker starts a math worker with the given runtime and script.
func StartWorker(runtime, script string) (*Worker, error) {
	if _, err := os.Stat(script); err != nil {
		return nil, errNoRuntime
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	command := exec.Command(runtime, script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	command.Stderr = io.Discard
	input, err := command.StdinPipe()
	if err == nil {
		var output io.ReadCloser
		if output, err = command.StdoutPipe(); err == nil {
			if err = command.Start(); err == nil {
				var process windows.Handle
				if process, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid)); err == nil {
					err = windows.AssignProcessToJobObject(job, process)
					windows.CloseHandle(process)
				}
				if err == nil {
					return &Worker{In: input, Out: output, Close: func() {
						// Closing the job ends the worker and anything it started.
						windows.CloseHandle(job)
						_ = command.Wait()
					}}, nil
				}
				_ = command.Process.Kill()
				_ = command.Wait()
			}
		}
	}
	windows.CloseHandle(job)
	return nil, err
}

//go:build windows

// Package processlifetime finishes a process after its application cleanup.
package processlifetime

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

// ExitAfterCleanup is only for main, after run has returned and every deferred
// save, resource restoration and owned-process wait has finished. Windows'
// ordinary ExitProcess invokes third-party DLL detach callbacks; an input
// method waiting on a foreign mutex can otherwise leave a windowless process
// alive indefinitely. TerminateProcess on ourselves skips those callbacks.
// Never use this to skip an application's unsaved-content or shutdown guards.
// https://learn.microsoft.com/windows/win32/api/processthreadsapi/nf-processthreadsapi-exitprocess
func ExitAfterCleanup(code int) {
	if err := windows.TerminateProcess(windows.CurrentProcess(), uint32(code)); err != nil {
		fmt.Fprintln(os.Stderr, "final process exit:", err)
	}
	os.Exit(code)
}

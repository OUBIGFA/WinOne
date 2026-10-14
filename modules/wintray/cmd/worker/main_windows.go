//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/signal"
	"winone/internal/processlifetime"
	"winone/modules/wintray/internal/app"
	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/worker"
)

func main() {
	processlifetime.ExitAfterCleanup(run())
}

// Run returns only after worker.Run has saved/restored resources and its
// deferred cleanup has completed. Final exit skips third-party DLL callbacks.
func run() int {
	if code := app.RunWorkerHelper(os.Args[1:]); code >= 0 {
		return code
	}
	defaultDir, err := config.AppDirWithError()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pipe := flag.String("pipe", "", "Host-assigned pipe")
	data := flag.String("data-dir", defaultDir, "module settings directory")
	autorun := flag.Bool("autorun", false, "run configured startup tasks")
	recoverOnly := flag.Bool("recover-only", false, "restore module resources without activating tasks")
	parent := flag.Uint("parent", 0, "owning Host process")
	flag.Parse()
	if *pipe == "" {
		fmt.Fprintln(os.Stderr, "missing pipe")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *parent != 0 {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(*parent))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		go func() {
			defer windows.CloseHandle(handle)
			_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
			cancel()
		}()
	}
	if err = worker.Run(ctx, *pipe, *data, *autorun, *recoverOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/signal"
	"winone/internal/diagnostics"
	"winone/internal/processlifetime"
	"winone/modules/always-on-top/worker"
)

func main() {
	// All resource restoration and deferred cancellation finishes in run before
	// skipping foreign DLL detach, which can otherwise strand a stopped worker.
	processlifetime.ExitAfterCleanup(run())
}

func run() int {
	pipe := flag.String("pipe", "", "Host-assigned pipe")
	data := flag.String("data-dir", "", "module directory")
	parent := flag.Uint("parent", 0, "Host process")
	flag.Parse()
	if *pipe == "" || *data == "" {
		fmt.Fprintln(os.Stderr, "missing pipe or data directory")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	reportFailure := func(cause error) {
		// Elevated launches cannot inherit the ordinary Host's stderr pipe.
		// Their failures use the same synchronized product file directly.
		if err := diagnostics.Record(diagnostics.ModuleDataRoot(*data, "always-on-top"), diagnostics.Event{Source: "always-on-top", Event: "worker-failed", Error: cause.Error()}); err != nil {
			fmt.Fprintln(os.Stderr, cause)
			fmt.Fprintln(os.Stderr, "WinOne diagnostic write failed:", err)
		}
	}
	if *parent != 0 {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(*parent))
		if err != nil {
			reportFailure(err)
			return 1
		}
		go func() {
			defer windows.CloseHandle(handle)
			_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
			cancel()
		}()
	}
	if err := worker.Run(ctx, *pipe, *data); err != nil {
		reportFailure(err)
		return 1
	}
	return 0
}

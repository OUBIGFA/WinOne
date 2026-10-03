//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/signal"
	"winone/modules/always-on-top/worker"
)

func main() {
	pipe := flag.String("pipe", "", "Host-assigned pipe")
	data := flag.String("data-dir", "", "module directory")
	parent := flag.Uint("parent", 0, "Host process")
	flag.Parse()
	if *pipe == "" || *data == "" {
		fmt.Fprintln(os.Stderr, "missing pipe or data directory")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *parent != 0 {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(*parent))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		go func() {
			defer windows.CloseHandle(handle)
			_, _ = windows.WaitForSingleObject(handle, windows.INFINITE)
			cancel()
		}()
	}
	if err := worker.Run(ctx, *pipe, *data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

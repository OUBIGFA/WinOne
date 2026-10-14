//go:build windows

// The pipe-driven recovery companion owns no application settings or UI.
package main

import (
	"flag"
	"fmt"
	"os"
	"winone/internal/diagnostics"
	"winone/internal/processlifetime"
	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/internal/traybox"
)

func main() { processlifetime.ExitAfterCleanup(run()) }
func run() int {
	flags := flag.NewFlagSet("WinOne.WinTray.Recovery", flag.ContinueOnError)
	watch := flags.Bool("watch-tray", false, "restore collected icons if the private pipe closes")
	directory := flags.String("data-dir", "", "Host-assigned module directory")
	if err := flags.Parse(os.Args[1:]); err != nil || !*watch || flags.NArg() != 0 {
		return 2
	}
	if err := config.SetWorkerDataDirectory(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := traybox.RunRecovery(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		writer, logErr := diagnostics.NewModuleWriter(*directory, "wintray")
		if logErr == nil {
			_, logErr = fmt.Fprintf(writer, "tray recovery failed: %v\n", err)
			closeErr := writer.Close()
			if logErr == nil {
				logErr = closeErr
			}
		}
		if logErr != nil {
			fmt.Fprintln(os.Stderr, "tray recovery logging:", logErr)
		}
		return 1
	}
	return 0
}

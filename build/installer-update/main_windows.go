//go:build windows

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"unicode/utf16"
	"winone/internal/distribution"
)

func main() {
	var request distribution.InstallerUpdate
	flag.StringVar(&request.Installation, "installation", "", "existing install")
	flag.StringVar(&request.Profile, "profile", "", "shared product configuration")
	flag.StringVar(&request.Staged, "staged", "", "installer-owned extracted payload")
	flag.StringVar(&request.RegistryKey, "registry-key", "", "current-user uninstall key")
	flag.StringVar(&request.ShortcutPath, "shortcut", "", "registered product shortcut")
	report := flag.String("report", "", "diagnostic output")
	progressWindow := flag.Uint64("progress-window", 0, "installer status label")
	progressPID := flag.Uint("progress-pid", 0, "installer status label owner")
	language := flag.Uint("language", 1033, "installer display language")
	flag.Parse()
	progress, progressErr := installerProgress(uintptr(*progressWindow), uint32(*progressPID), *language)
	request.Progress = progress
	if err := func() error {
		if progressErr != nil {
			return progressErr
		}
		return distribution.RunInstallerUpdate(request)
	}(); err != nil {
		if *report != "" {
			// Unicode NSIS reads UTF-16LE explicitly, independent of the user's
			// Windows ANSI code page. Do not lose localized diagnostic text.
			units := utf16.Encode([]rune(err.Error()))
			data := make([]byte, len(units)*2)
			for i, v := range units {
				binary.LittleEndian.PutUint16(data[i*2:], v)
			}
			if writeErr := os.WriteFile(*report, data, 0600); writeErr != nil {
				fmt.Fprintln(os.Stderr, writeErr)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

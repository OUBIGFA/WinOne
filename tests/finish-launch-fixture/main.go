//go:build windows

// A finish-page test launches this owned executable. It proves that the normal
// runtime lease can be acquired after the installer releases maintenance.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"winone/internal/maintenance"
)

func main() {
	marker, profile := os.Getenv("WINONE_FINISH_MARKER"), os.Getenv("WINONE_FINISH_PROFILE")
	if !filepath.IsAbs(marker) || !filepath.IsAbs(profile) {
		os.Exit(2)
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(3)
	}
	lease, err := maintenance.AcquireRuntimeForInstallation(profile, filepath.Dir(executable))
	message := ""
	if err != nil {
		message = err.Error()
	} else {
		defer lease.Close()
	}
	data, _ := json.Marshal(map[string]any{"runtimeAdmitted": err == nil, "error": message, "pid": os.Getpid()})
	if os.WriteFile(marker, data, 0600) != nil {
		os.Exit(4)
	}
}

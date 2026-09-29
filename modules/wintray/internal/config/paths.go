package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	appDirName = "WinTray"
)

var workerDataDirectory string

// SetWorkerDataDirectory is called once before creating any worker services.
func SetWorkerDataDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("worker data directory must be absolute")
	}
	workerDataDirectory = filepath.Clean(path)
	return nil
}

func AppDirWithError() (string, error) {
	if workerDataDirectory != "" {
		return workerDataDirectory, nil
	}
	base, err := appDataBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinOne", "modules", "wintray"), nil
}

func SettingsPathWithError() (string, error) {
	dir, err := AppDirWithError()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func LogPathWithError() (string, error) {
	dir, err := AppDirWithError()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wintray.log"), nil
}

func appDataBaseDir() (string, error) {
	if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
		return localAppData, nil
	}

	if cfgDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(cfgDir) != "" {
		if runtime.GOOS == "windows" {
			roaming := filepath.Join("AppData", "Roaming")
			if strings.HasSuffix(strings.ToLower(cfgDir), strings.ToLower(roaming)) {
				localDir := filepath.Join(filepath.Dir(cfgDir), "Local")
				return localDir, nil
			}
		}
		return cfgDir, nil
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Local"), nil
		}
		return filepath.Join(home, ".config"), nil
	}

	return "", errors.New("unable to resolve a stable app data directory")
}

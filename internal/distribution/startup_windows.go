//go:build windows

package distribution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
	"winone/internal/maintenance"
	"winone/internal/startupprobe"
)

// CheckNativeUpgrade runs the target's Host/configuration, worker and WebView2
// checks under the caller's maintenance lease. It never promotes a pending
// configuration or migrates live settings. Scratch is caller-owned diagnostics.
func CheckNativeUpgrade(installation, profile, scratch string) error {
	before, err := readUpgradeProfile(profile)
	if err != nil {
		return err
	}
	if err = maintenance.RequireStopped(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	host := filepath.Join(installation, "WinOne.Host.exe")
	if err = startupprobe.Check(ctx, host); err != nil {
		return err
	}
	if before != nil {
		if err = startupprobe.CheckConfiguration(ctx, host, before); err != nil {
			return err
		}
	}
	if err = startupprobe.CheckWorkers(ctx, installation, scratch); err != nil {
		return err
	}
	if err = startupprobe.CheckDesktop(ctx, filepath.Join(installation, "WinOne.exe"), scratch); err != nil {
		return err
	}
	after, err := readUpgradeProfile(profile)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) || (before == nil) != (after == nil) {
		return errors.New("configuration changed during startup checks")
	}
	return maintenance.RequireStopped()
}

// Missing canonical data is only acceptable for a never-configured profile.
// Known legacy/projection files are not silently treated as fresh defaults.
func readUpgradeProfile(directory string) ([]byte, error) {
	root, err := openUpgradeRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := openInstalledFile(root, "configuration.json")
	if errors.Is(err, os.ErrNotExist) {
		for _, name := range []string{"settings.json", "module-state.json", "modules/capture/settings.json", "modules/capture/managed-settings.json", "modules/always-on-top/settings.json", "modules/always-on-top/managed-settings.json", "modules/wintray/settings.json", "modules/wintray/managed-settings.json"} {
			_, err := root.Lstat(name)
			if err == nil {
				return nil, errors.New("complete legacy configuration migration before upgrading")
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > 8<<20 {
		return nil, errors.New("invalid upgrade configuration size")
	}
	return data, nil
}

//go:build windows

package distribution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"winone/internal/maintenance"
	"winone/internal/modulestate"
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
	if err = maintenance.RequireStoppedAt(installation); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// CheckWorkers validates the target Host handshake and its reported catalog.
	if before != nil {
		if err = CheckNativeUpgradeConfiguration(installation, profile, scratch); err != nil {
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
	return maintenance.RequireStoppedAt(installation)
}

// CheckNativeUpgradeConfiguration runs on the verified staged Host before any
// payload replacement. Existing quarantined data is retained; only actionable
// incompatibility or unfinished transactions reject the update.
func CheckNativeUpgradeConfiguration(installation, profile, scratch string) error {
	data, err := readUpgradeProfile(profile)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	if err = maintenance.RequireStoppedAt(installation); err != nil {
		return err
	}
	before, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, before.Configuration) {
		return errors.New("profile changed before upgrade validation")
	}
	if before.ModuleState != nil {
		state, err := modulestate.DecodeSnapshot(before.ModuleState)
		if err != nil {
			return err
		}
		for id, record := range state.Modules {
			if record.PendingStop {
				return fmt.Errorf("模块 %s 尚未完成资源恢复，请先恢复再升级", id)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	executable := filepath.Join(installation, "WinOne.Host.exe")
	if before.RequiresPackageCheck() {
		report, err := startupprobe.CheckProfileConfiguration(ctx, executable, profile, data)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(scratch) {
			return errors.New("upgrade probe scratch must be absolute")
		}
		if err = os.MkdirAll(scratch, 0700); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(scratch, "profile-check.json"), encoded, 0600); err != nil {
			return err
		}
	} else if err = startupprobe.CheckConfiguration(ctx, executable, data); err != nil {
		return err
	}
	after, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil {
		return err
	}
	if after.Digest() != before.Digest() {
		return errors.New("profile changed during upgrade validation")
	}
	return nil
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
		for _, name := range []string{"settings.json", "module-state.json", "packages/.winone-module-registry.json", "modules/capture/settings.json", "modules/capture/managed-settings.json", "modules/always-on-top/settings.json", "modules/always-on-top/managed-settings.json", "modules/wintray/settings.json", "modules/wintray/managed-settings.json", "modules/selection/settings.json", "modules/selection/managed-settings.json"} {
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

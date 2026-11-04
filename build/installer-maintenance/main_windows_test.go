//go:build windows

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"winone/internal/installguard"
)

func TestMaintenanceArgumentsPreserveTheInstallerProtocol(t *testing.T) {
	root := t.TempDir()
	installation, profile := filepath.Join(root, "installed"), filepath.Join(root, "profile")
	args := []string{"--guard-installation", installation, "--data-dir", profile, "--installer-pid", "123", "--installer-ready", `Local\WinOne-ready`, "--installer-release", `Local\WinOne-release`, "--pipe", `\\.\pipe\WinOne-maintenance-test`, "--prepare-uninstall"}
	got, err := parseArguments(args)
	want := installguard.Request{Installation: installation, Profile: profile, OwnerPID: 123, ReadyEvent: `Local\WinOne-ready`, ReleaseEvent: `Local\WinOne-release`, Pipe: `\\.\pipe\WinOne-maintenance-test`, PrepareUninstall: true}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("maintenance protocol changed: got %+v, error %v; want %+v", got, err, want)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("argument parsing touched the profile: %v", err)
	}
}

func TestMaintenanceArgumentsNeverAdmitHostRuntimeModes(t *testing.T) {
	root := t.TempDir()
	base := []string{"--guard-installation", filepath.Join(root, "installed"), "--data-dir", filepath.Join(root, "profile"), "--installer-pid", "123", "--installer-ready", `Local\WinOne-ready`}
	for _, mode := range []string{"--headless", "--autorun", "--standard-session", "--check-startup=nonce", "--check-profile=nonce", "--desktop-control", "--unknown"} {
		t.Run(mode, func(t *testing.T) {
			args := append(append([]string{}, base...), mode)
			if _, err := parseArguments(args); err == nil {
				t.Fatalf("maintenance accepted Host/runtime mode %s", mode)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "profile")); !os.IsNotExist(err) {
		t.Fatalf("rejected runtime mode touched the profile: %v", err)
	}
}

func TestMaintenanceArgumentsRequireAnExplicitOwnerAndProfile(t *testing.T) {
	root := t.TempDir()
	base := []string{"--guard-installation", filepath.Join(root, "installed"), "--data-dir", filepath.Join(root, "profile"), "--installer-pid", "123", "--installer-ready", `Local\WinOne-ready`}
	for _, scenario := range []struct {
		name string
		args []string
	}{
		{"empty", nil},
		{"uninstall-alone", []string{"--prepare-uninstall"}},
		{"no-profile", append(append([]string{}, base[:2]...), base[4:]...)},
		{"relative-installation", append([]string{"--guard-installation", "relative"}, base[2:]...)},
		{"relative-profile", append(append([]string{}, base[:2]...), append([]string{"--data-dir", "relative"}, base[4:]...)...)},
		{"zero-owner", append(append([]string{}, base...), "--installer-pid=0")},
		{"overflow-owner", append(append([]string{}, base...), "--installer-pid=4294967296")},
		{"negative-owner", append(append([]string{}, base...), "--installer-pid=-1")},
		{"empty-ready", append(append([]string{}, base...), "--installer-ready=")},
		{"positional", append(append([]string{}, base...), "unexpected")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := parseArguments(scenario.args); err == nil {
				t.Fatalf("invalid maintenance request was accepted: %v", scenario.args)
			}
		})
	}
}

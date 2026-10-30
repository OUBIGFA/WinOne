//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/maintenance"
	"winone/internal/settings"
	"winone/internal/startupprobe"
)

func TestStartupProbeReportsWithoutStartingRuntime(t *testing.T) {
	var output bytes.Buffer
	if err := writeStartupProbe(&output, "test-nonce"); err != nil {
		t.Fatal(err)
	}
	var report startupprobe.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Nonce != "test-nonce" || report.PID <= 0 || len(report.Modules) != 7 {
		t.Fatal(report)
	}
	if err := report.Validate(report.PID, report.Executable, "wrong-nonce"); err == nil {
		t.Fatal("unrelated response accepted")
	}
}

func probeConfiguration(t *testing.T) []byte {
	t.Helper()
	directory := t.TempDir()
	store, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	global, _ := json.Marshal(store.Snapshot())
	doc := configuration.Document{SchemaVersion: 1, Values: map[string]json.RawMessage{"host": global}, ModuleEnabled: map[string]bool{}}
	for _, adapter := range []configuration.ModuleAdapter{aotConfiguration(directory, nil), captureConfiguration(directory, nil), wintrayConfiguration(directory, nil, nil)} {
		raw, err := adapter.Load()
		if err != nil {
			t.Fatal(err)
		}
		doc.Values[adapter.ID], doc.ModuleEnabled[adapter.ID] = raw, false
	}
	data, err := json.Marshal(map[string]any{"committed": doc})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConfigurationProbeValidatesWithoutLoadingProfile(t *testing.T) {
	data := probeConfiguration(t)
	var output bytes.Buffer
	if err := writeConfigurationProbe(&output, bytes.NewReader(data), "test-nonce"); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigurationProbe(&output, bytes.NewReader([]byte(`{}`)), "test-nonce"); err == nil {
		t.Fatal("bad configuration accepted")
	}
}

func TestNativeConfigurationProbe(t *testing.T) {
	executable := os.Getenv("WINONE_HOST_EXE")
	if executable == "" {
		t.Skip("set WINONE_HOST_EXE for native configuration probe")
	}
	image, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	installation, local := t.TempDir(), t.TempDir()
	executable = filepath.Join(installation, "WinOne.Host.exe")
	if err = os.WriteFile(executable, image, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	pending := filepath.Join(installation, ".winone-pending")
	if err = os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	data := probeConfiguration(t)
	path := filepath.Join(profile, "configuration.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := startupprobe.CheckConfiguration(context.Background(), executable, data); err != nil {
		t.Fatal(err)
	}
	if err := startupprobe.CheckConfiguration(context.Background(), executable, []byte(`{}`)); err == nil {
		t.Fatal("invalid configuration accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("configuration changed", err)
	}
	marker, err := os.ReadFile(pending)
	if err != nil || string(marker) != "pending" {
		t.Fatal("pending marker changed", err)
	}
}

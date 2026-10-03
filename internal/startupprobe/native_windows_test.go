//go:build windows

package startupprobe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"winone/internal/maintenance"
)

func TestNativeHostProbeUnderMaintenance(t *testing.T) {
	source := os.Getenv("WINONE_HOST_EXE")
	if source == "" {
		t.Skip("set WINONE_HOST_EXE for native Host probe")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	installation, local := t.TempDir(), t.TempDir()
	executable := filepath.Join(installation, "WinOne.Host.exe")
	if err := os.WriteFile(executable, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	configuration := filepath.Join(profile, "configuration.json")
	if err := os.WriteFile(configuration, []byte("deliberately invalid live configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(installation, ".winone-pending")
	if err := os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	before, err := os.ReadDir(profile)
	if err != nil {
		t.Fatal(err)
	}
	var beforeNames []string
	for _, entry := range before {
		beforeNames = append(beforeNames, entry.Name())
	}
	for i := 0; i < 2; i++ {
		if err := Check(context.Background(), executable); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"--headless"}, {"--check-startup", "nonce", "--autorun"}, {"--check-startup="}} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := exec.CommandContext(ctx, executable, args...).Run()
		expired := ctx.Err()
		cancel()
		if err == nil || expired != nil {
			t.Fatalf("entry was not promptly refused: %v: %v", args, err)
		}
	}
	for path, want := range map[string]string{configuration: "deliberately invalid live configuration", pending: "pending"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("probe changed %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(profile)
	var afterNames []string
	for _, entry := range entries {
		afterNames = append(afterNames, entry.Name())
	}
	if err != nil || !reflect.DeepEqual(beforeNames, afterNames) {
		t.Fatalf("unexpected profile files: %v: %v", entries, err)
	}
}

//go:build windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/maintenance"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/transport"
)

// Explicit integration runner only: the real Host handlers and configuration
// store are exercised over Windows pipes. Lifecycle hooks own no OS resources,
// so the test never registers shortcuts, starts programs or changes sign-in.
func TestRustDesktopConfigurationRoundtrip(t *testing.T) {
	executable := os.Getenv("WINONE_RUST_CONFIGURATION_TEST_EXE")
	if executable == "" {
		t.Skip("run build/test-desktop-rust.ps1 with -GoExe")
	}
	directory := t.TempDir()
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	gates := map[string]*modules.Gate{}
	for _, manifest := range catalog.Builtins() {
		gate, err := modules.NewGate(manifest.ID, manifest.Name.Chinese, state, modules.LifecycleHooks{
			Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil },
			Status: func() host.Module { return host.Module{ID: manifest.ID, State: "ready"} },
			Handle: func(r host.Request) host.Response {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "test", Message: "feature actions are disabled"}}
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		gates[manifest.ID] = gate
	}
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, func() map[string]*modules.Gate { return gates }),
		aotConfiguration(directory, get), captureConfiguration(directory, get), wintrayConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	registerDiagnostics(service, directory)
	control := modules.NewControl(state, gates, catalog.Builtins)
	service.RegisterModule("modules", func() host.Module { return host.Module{ID: "modules", State: "ready"} }, moduleConfigurationHandler(store, state, control))
	service.RegisterModule("profiles", func() host.Module { return host.Module{ID: "profiles", State: "ready"} }, profileHandler(store, state))
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-rust-configuration-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	done := make(chan error, 1)
	go func() { done <- transport.Serve(ctx, listener, host.NewConfigurationQueue(service)) }()
	defer func() { cancel(); listener.Close(); <-done }()
	command := exec.CommandContext(ctx, executable, "--ignored", "--exact", "worker::tests::real_host_module_and_profile_roundtrip", "--nocapture")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Env = append(os.Environ(), "WINONE_RUST_CONFIGURATION_PIPE="+pipe, "WINONE_RUST_CONFIGURATION_DIRECTORY="+directory)
	expectedPipe, err := transport.PipeName()
	if err != nil {
		t.Fatal(err)
	}
	command.Env = append(command.Env, "WINONE_RUST_EXPECTED_PIPE="+expectedPipe)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Rust configuration roundtrip: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
	if store.Snapshot().Theme != "dark" || store.CommittedDocument().ModuleEnabled["capture"] {
		t.Fatal("import did not restore the exported settings and module switch")
	}
}

func TestRustDesktopMaintenanceLocks(t *testing.T) {
	executable := os.Getenv("WINONE_RUST_CONFIGURATION_TEST_EXE")
	if executable == "" {
		t.Skip("run build/test-desktop-rust.ps1 with -GoExe")
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := t.TempDir()
	installation := filepath.Join(t.TempDir(), "安装-İ")
	if err := os.Mkdir(installation, 0700); err != nil {
		t.Fatal(err)
	}
	command := func(mode string) (*exec.Cmd, context.CancelFunc) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, executable, "--ignored", "--exact", "startup::tests::real_maintenance_lock_probe", "--nocapture")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Env = append(os.Environ(), "WINONE_RUST_LOCK_PROFILE="+profile, "WINONE_RUST_LOCK_INSTALLATION="+installation, "WINONE_RUST_LOCK_LOCAL="+local, "WINONE_RUST_LOCK_MODE="+mode)
		return cmd, cancel
	}
	blocked := func() {
		cmd, cancel := command("blocked")
		defer cancel()
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Rust lease: %v\n%s", err, output)
		}
	}
	installationLease, err := maintenance.AcquireMaintenanceForInstallation(t.TempDir(), installation)
	if err != nil {
		t.Fatal(err)
	}
	blocked()
	installationLease.Close()
	profileLease, err := maintenance.AcquireMaintenance(profile)
	if err != nil {
		t.Fatal(err)
	}
	blocked()
	profileLease.Close()
	marker := filepath.Join(installation, ".winone-pending")
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	blocked()
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	cmd, cancel := command("hold")
	defer cancel()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "LOCK_READY") {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("Rust lease did not become ready: %s", stderr.String())
	}
	if lease, err := maintenance.AcquireMaintenance(profile); err == nil {
		lease.Close()
		t.Fatal("Rust profile lease did not block maintenance")
	}
	if lease, err := maintenance.AcquireMaintenanceForInstallation(t.TempDir(), installation); err == nil {
		lease.Close()
		t.Fatal("Rust installation lease did not block maintenance")
	}
	stdin.Write([]byte{1})
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err, stderr.String())
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal("Rust did not release its leases", err)
	}
	lease.Close()
}

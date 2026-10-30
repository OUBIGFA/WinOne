//go:build windows

package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/maintenance"
	"winone/internal/transport"
)

func TestInstallerProcessOwner(t *testing.T) {
	if os.Getenv("WINONE_INSTALLER_TEST_OWNER") != "1" {
		return
	}
	fmt.Println("ready")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestInstallerGuardHoldsUntilInstallerExit(t *testing.T) {
	exe := os.Getenv("WINONE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_HOST_EXE for guard integration")
	}
	copyPath := filepath.Join(t.TempDir(), "WinOne.Maintenance.exe")
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(copyPath, data, 0700); err != nil {
		t.Fatal(err)
	}
	owner := exec.Command(os.Args[0], "-test.run=^TestInstallerProcessOwner$")
	owner.Env = append(os.Environ(), "WINONE_INSTALLER_TEST_OWNER=1")
	in, _ := owner.StdinPipe()
	out, _ := owner.StdoutPipe()
	if err = owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); owner.Process.Kill(); owner.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	name := fmt.Sprintf("Local\\WinOne-guard-%d", time.Now().UnixNano())
	key, _ := windows.UTF16PtrFromString(name)
	event, err := windows.CreateEvent(nil, 1, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	profile := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-guard-%d`, time.Now().UnixNano())
	guard := exec.Command(copyPath, "--guard-installation", filepath.Dir(exe), "--installer-pid", strconv.Itoa(owner.Process.Pid), "--installer-ready", name, "--prepare-uninstall", "--pipe", pipe, "--data-dir", profile)
	var diagnostic bytes.Buffer
	guard.Stdout, guard.Stderr = &diagnostic, &diagnostic
	if err = guard.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { guard.Process.Kill(); guard.Wait() }()
	if state, err := windows.WaitForSingleObject(event, 15000); err != nil || state != windows.WAIT_OBJECT_0 {
		_ = guard.Process.Kill()
		_ = guard.Wait()
		t.Fatal("guard not ready", state, err, diagnostic.String())
	}
	if lease, err := maintenance.AcquireRuntimeForInstallation(t.TempDir(), filepath.Dir(exe)); err == nil {
		lease.Close()
		t.Fatal("guard released lock while installer alive")
	}
	in.Close()
	if err = owner.Wait(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- guard.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("guard did not exit with installer")
	}
	lease, err := maintenance.AcquireRuntimeForInstallation(profile, filepath.Dir(exe))
	if err != nil {
		t.Fatal("guard retained lock", err)
	}
	lease.Close()
}

// The isolated profile cannot modify real startup tasks. This verifies the
// actual CLI, pipe exclusion, worker restoration and preference preservation.
func TestHostUninstallPreparation(t *testing.T) {
	exe := os.Getenv("WINONE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_HOST_EXE for packaged maintenance checks")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "user-note.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-maintenance-%d`, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	owner := exec.CommandContext(ctx, exe, "--headless", "--pipe", pipe, "--data-dir", dir)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if owner.Process != nil {
			_ = owner.Process.Kill()
			_ = owner.Wait()
		}
	}()
	for {
		if _, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"}); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(25 * time.Millisecond)
	}
	args := []string{"--prepare-uninstall", "--pipe", pipe, "--data-dir", dir}
	if err := exec.CommandContext(ctx, exe, args...).Run(); err == nil {
		t.Fatal("maintenance admitted with running Host")
	}
	// A different pipe must not bypass the running-process check.
	args[2] = pipe + "-other"
	if err := exec.CommandContext(ctx, exe, args...).Run(); err == nil {
		t.Fatal("alternate pipe bypassed process guard")
	}
	_ = owner.Process.Kill()
	_ = owner.Wait()
	owner.Process = nil
	args[2] = pipe
	// An installer-held lease must reject both normal entry points before
	// either can create a window or start another Host.
	lease, err := maintenance.AcquireMaintenanceForInstallation(dir, filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	for _, target := range []struct {
		path string
		args []string
	}{
		{exe, []string{"--headless", "--pipe", pipe, "--data-dir", t.TempDir()}},
		{filepath.Join(filepath.Dir(exe), "WinOne.exe"), []string{"--pipe", pipe, "--data-dir", t.TempDir()}},
	} {
		// A modal startup error is not an explicit maintenance rejection: it
		// keeps the executable locked until someone dismisses the dialog.
		entryCtx, entryCancel := context.WithTimeout(ctx, 5*time.Second)
		err := exec.CommandContext(entryCtx, target.path, target.args...).Run()
		entryContextErr := entryCtx.Err()
		entryCancel()
		if entryContextErr != nil {
			t.Fatal("runtime did not exit promptly after maintenance rejection", target.path, entryContextErr)
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatal("runtime did not explicitly reject maintenance lock", target.path, err)
		}
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "configuration.json")
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if output, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
			t.Fatalf("prepare: %v %s", err, output)
		}
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("maintenance changed Host settings", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("maintenance removed user file", err)
	}
}

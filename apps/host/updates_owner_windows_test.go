//go:build windows

package main

import (
	"context"
	"golang.org/x/sys/windows"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"winone/internal/updates"
)

type updateRoundTrip func(*http.Request) (*http.Response, error)

func (f updateRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestUpdatesOwnerProcessExitStopsTheNetworkRequest(t *testing.T) {
	if os.Getenv("WINONE_UPDATE_OWNER_CHILD") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ownerPath := filepath.Join(directory, "WinOne.exe")
	if err = os.WriteFile(ownerPath, binary, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, ownerPath, "-test.run=^TestUpdatesOwnerProcessExitStopsTheNetworkRequest$")
	command.Env = append(os.Environ(), "WINONE_UPDATE_OWNER_CHILD=1")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = command.Process.Kill(); _ = command.Wait() }()
	started := make(chan struct{})
	checker := updates.Checker{Repository: "example/app", CurrentVersion: "1.0.0", Transport: updateRoundTrip(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	control := newUpdateRPC(ctx, filepath.Join(directory, "WinOne.Host.exe"), checker)
	defer control.close()
	pid := uint32(command.Process.Pid)
	updateSnapshot(t, updateRequest(t, control, pid, "check", "owner-exit"))
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("network check did not start")
	}
	_ = input.Close()
	if err = command.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		result := updateSnapshot(t, updateRequest(t, control, pid, "status", "owner-exit"))
		if result.State != "checking" {
			if result.ErrorCode != "cancelled" {
				t.Fatal(result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dead UI retained its network request")
		}
		time.Sleep(time.Millisecond)
	}
}

//go:build windows

package startupprobe

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"winone/internal/host"
	"winone/internal/selection"
	"winone/internal/transport"
)

// CheckWorkers probes verified bundle workers using fresh, isolated settings.
// Scratch must be an existing diagnostic directory outside the installation.
// It retains the generated worker data for diagnosis; the caller owns cleanup.
// No user actions, login programs or live configuration are supplied.
func CheckWorkers(ctx context.Context, installation, scratch string) error {
	if !filepath.IsAbs(installation) || !filepath.IsAbs(scratch) {
		return errors.New("worker probe paths must be absolute")
	}
	installation, err := filepath.EvalSymlinks(installation)
	if err != nil {
		return err
	}
	scratch, err = filepath.EvalSymlinks(scratch)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(installation, scratch)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("worker probe data must be outside installation")
	}
	// Probe the target version's validated catalog. Older rollback bundles
	// legitimately have three modules; the updater's own catalog is not theirs.
	var target Report
	if err := checkExecutable(ctx, filepath.Join(installation, "WinOne.Host.exe"), nil, false, "", "", &target); err != nil {
		return err
	}
	for _, manifest := range target.Modules {
		if err := ctx.Err(); err != nil {
			return err
		}
		if manifest.ID == "quick-assistant" {
			if err := Check(ctx, filepath.Join(installation, filepath.FromSlash(manifest.Worker))); err != nil {
				return fmt.Errorf("quick assistant startup probe: %w", err)
			}
			continue
		}
		if manifest.ID == "selection" {
			// This component is a one-shot stdin/stdout reader, not an RPC worker.
			// A zero source checks launch, protocol and exit without inspecting any
			// user window. Reuse the real reader's timeout and process-job ownership.
			value, err := selection.ReadSelection(ctx, filepath.Join(installation, filepath.FromSlash(manifest.Worker)), selection.Event{Keyboard: true})
			if err != nil {
				return fmt.Errorf("selection startup probe: %w", err)
			}
			if value.Status != "stale" || value.Text != "" || len(value.Rectangles) != 0 {
				return errors.New("selection startup probe returned unexpected content")
			}
			continue
		}
		directory, err := os.MkdirTemp(scratch, manifest.ID+"-probe-")
		if err != nil {
			return err
		}
		if err := checkWorker(ctx, manifest.ID, filepath.Join(installation, filepath.FromSlash(manifest.Worker)), directory); err != nil {
			return fmt.Errorf("%s startup probe: %w", manifest.ID, err)
		}
	}
	return nil
}

func checkWorker(parent context.Context, id, executable, directory string) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	pipe, err := transport.PipeName()
	if err != nil {
		return err
	}
	pipe += "-worker-probe-" + rand.Text()
	cmd := exec.CommandContext(ctx, executable, "--pipe", pipe, "--data-dir", directory, "--parent", strconv.Itoa(os.Getpid()))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	// Workers are owned by this probe, never attached to existing user sessions.
	// Cancellation reaps only this child; empty settings make forced recovery safe.
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	defer func() { cancel(); <-done }()
	call := func(method string) (host.Response, error) {
		timeout := 500 * time.Millisecond
		if method == "shutdown" {
			timeout = 5 * time.Second
		}
		return transport.CallWithTimeout(ctx, pipe, host.Request{Version: 1, ID: "startup-probe", Method: method}, timeout)
	}
	for {
		response, err := call("snapshot")
		if err == nil {
			if response.Error != nil {
				return errors.New(response.Error.Message)
			}
			ready, err := workerReady(id, cmd.Process.Pid, response.Result)
			if err != nil {
				return err
			}
			if ready {
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return fmt.Errorf("worker exited before readiness: %v", waitErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
	response, err := call("shutdown")
	if err != nil {
		return fmt.Errorf("worker shutdown: %w", err)
	}
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return waitErr
	}
}

// WinTray exposes its pipe before asynchronous activation finishes; its PID
// alone must not be mistaken for successful initialization.
func workerReady(id string, pid int, data json.RawMessage) (bool, error) {
	var state struct {
		PID                 int    `json:"processId"`
		Initialization      string `json:"initializationState"`
		InitializationError string `json:"initializationError"`
		RecoveryError       string `json:"recoveryError"`
		BackgroundError     string `json:"backgroundError"`
		StartupError        string `json:"startupError"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	if state.PID != pid || pid <= 0 {
		return false, errors.New("worker probe identity mismatch")
	}
	for _, problem := range []string{state.InitializationError, state.RecoveryError, state.BackgroundError, state.StartupError} {
		if problem != "" {
			return false, errors.New(problem)
		}
	}
	if id == "wintray" {
		switch state.Initialization {
		case "starting":
			return false, nil
		case "ready":
			return true, nil
		default:
			return false, errors.New("invalid WinTray initialization state")
		}
	}
	return true, nil
}

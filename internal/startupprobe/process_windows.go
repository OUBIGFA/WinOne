//go:build windows

package startupprobe

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type limitedOutput struct {
	buffer bytes.Buffer
	limit  int
}

// Returning an error also bounds malicious or broken probe output.
func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("startup probe output too large")
	}
	return b.buffer.Write(p)
}

// Check starts a previously verified Host executable in read-only probe mode.
// The caller retains the maintenance lease and pending marker. This verifies
// Host launch only, not worker/UI health or compatibility of live settings.
// CommandContext and WaitDelay follow the Go os/exec cancellation contract:
// only this owned probe process is terminated on timeout, then reaped by Wait.
func Check(ctx context.Context, executable string) error {
	return checkExecutable(ctx, executable, nil, false, "")
}

// CheckConfiguration verifies the supplied committed configuration with the
// target version's validators; bytes never become a live on-disk profile.
func CheckConfiguration(ctx context.Context, executable string, data []byte) error {
	if len(data) == 0 || len(data) > 8<<20 {
		return errors.New("invalid configuration probe size")
	}
	return checkExecutable(ctx, executable, append([]byte(nil), data...), true, "")
}

// CheckDesktop verifies the actual WebView2, production assets and Wails bridge
// using a fresh cache below scratch, without activating the user's Host.
func CheckDesktop(ctx context.Context, executable, scratch string) error {
	if !filepath.IsAbs(scratch) {
		return errors.New("desktop probe requires an absolute scratch path")
	}
	return checkExecutable(ctx, executable, nil, false, scratch)
}

func checkExecutable(ctx context.Context, executable string, configuration []byte, checkConfiguration bool, scratch string) error {
	if !filepath.IsAbs(executable) {
		return errors.New("startup probe requires an absolute executable path")
	}
	timeout := 15 * time.Second
	if scratch != "" {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	nonce := rand.Text()
	flagName := "--check-startup"
	if checkConfiguration {
		flagName = "--check-configuration"
	}
	if scratch != "" {
		flagName = "--check-ui"
	}
	args := []string{flagName, nonce}
	if scratch != "" {
		args = append(args, "--probe-root", scratch)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	if checkConfiguration {
		cmd.Stdin = bytes.NewReader(configuration)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	cmd.WaitDelay = time.Second
	output := &limitedOutput{limit: 32 * 1024}
	stderr := &limitedOutput{limit: 4096}
	cmd.Stdout, cmd.Stderr = output, stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Host probe: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Host probe failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	decoder := json.NewDecoder(&output.buffer)
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return fmt.Errorf("read Host probe: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing Host probe output")
	}
	if err := report.Validate(pid, executable, nonce); err != nil {
		return err
	}
	if checkConfiguration {
		sum := sha256.Sum256(configuration)
		if report.ConfigurationSHA256 != hex.EncodeToString(sum[:]) {
			return errors.New("configuration probe result mismatch")
		}
	}
	if scratch != "" && !report.UIReady {
		return errors.New("desktop probe readiness missing")
	}
	return nil
}

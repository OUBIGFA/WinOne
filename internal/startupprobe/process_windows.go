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
	"golang.org/x/sys/windows"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
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
	return checkExecutable(ctx, executable, nil, false, "", "", nil)
}

// CheckConfiguration verifies the supplied committed configuration with the
// target version's validators; bytes never become a live on-disk profile.
func CheckConfiguration(ctx context.Context, executable string, data []byte) error {
	if len(data) == 0 || len(data) > 8<<20 {
		return errors.New("invalid configuration probe size")
	}
	return checkExecutable(ctx, executable, append([]byte(nil), data...), true, "", "", nil)
}

// CheckProfileConfiguration additionally binds module intent and registry data.
// The target validates packages with its own trust and compatibility policy.
func CheckProfileConfiguration(ctx context.Context, executable, profile string, data []byte) (Report, error) {
	var report Report
	if len(data) == 0 || len(data) > 8<<20 {
		return report, errors.New("invalid configuration probe size")
	}
	before, err := ReadProfileSnapshot(profile)
	if err != nil {
		return report, err
	}
	if !bytes.Equal(before.Configuration, data) {
		return report, errors.New("configuration changed before profile probe")
	}
	if err = checkExecutable(ctx, executable, append([]byte(nil), data...), true, "", profile, &report); err != nil {
		return report, err
	}
	if report.ProfileSHA256 != before.Digest() {
		return report, errors.New("profile probe result mismatch")
	}
	after, err := ReadProfileSnapshot(profile)
	if err != nil {
		return report, err
	}
	if after.Digest() != before.Digest() {
		return report, errors.New("profile changed during probe")
	}
	return report, nil
}

// CheckDesktop verifies the actual WebView2, production assets and Wails bridge
// using a fresh cache below scratch, without activating the user's Host.
func CheckDesktop(ctx context.Context, executable, scratch string) error {
	if !filepath.IsAbs(scratch) {
		return errors.New("desktop probe requires an absolute scratch path")
	}
	return checkExecutable(ctx, executable, nil, false, scratch, "", nil)
}

func checkExecutable(ctx context.Context, executable string, configuration []byte, checkConfiguration bool, scratch, profile string, observed *Report) error {
	if !filepath.IsAbs(executable) {
		return errors.New("startup probe requires an absolute executable path")
	}
	timeout := 15 * time.Second
	if scratch != "" {
		timeout = 30 * time.Second
	}
	if profile != "" {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	nonce := rand.Text()
	flagName := "--check-startup"
	if checkConfiguration {
		flagName = "--check-configuration"
	}
	if profile != "" {
		flagName = "--check-profile"
	}
	if scratch != "" {
		flagName = "--check-ui"
	}
	args := []string{flagName, nonce}
	if scratch != "" {
		args = append(args, "--probe-root", scratch)
	}
	if profile != "" {
		args = append(args, "--profile-root", profile)
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
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Host probe: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		// These owned read-only probes report validation errors on bounded stderr.
		// Include the probe kind so UI failures are not misreported as Host launch.
		return fmt.Errorf("%s probe failed: %w: %s", flagName, err, strings.TrimSpace(stderr.buffer.String()))
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
	if scratch != "" && report.BrowserProcessID != 0 {
		uiElapsed := time.Since(started)
		remaining := time.Duration(0)
		if deadline, ok := ctx.Deadline(); ok {
			remaining = max(0, time.Until(deadline))
		}
		waiting := time.Now()
		if err := waitProbeBrowser(ctx, report); err != nil {
			path, recordErr := recordDesktopFailure(scratch, report, uiElapsed, time.Since(waiting), remaining, strings.TrimSpace(stderr.buffer.String()), err)
			if recordErr != nil {
				return errors.Join(err, fmt.Errorf("write desktop probe diagnostics: %w", recordErr))
			}
			return fmt.Errorf("%w; browser PID=%d, diagnostics=%s", err, report.BrowserProcessID, path)
		}
	}
	if observed != nil {
		*observed = report
	}
	return nil
}

// WebView2 may release its browser/cache only after the UI process has exited.
// Wait in the checker, using the SDK-reported PID and creation time. Old probes
// without this optional identity keep their prior compatible handshake.
func waitProbeBrowser(ctx context.Context, report Report) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, report.BrowserProcessID)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open probe browser: %w", err)
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return err
	}
	if uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) != report.BrowserCreated {
		return nil
	}
	for {
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("probe browser did not exit: %w", err)
		}
		state, err := windows.WaitForSingleObject(h, 50)
		if err != nil {
			return err
		}
		if state == windows.WAIT_OBJECT_0 {
			return nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("unexpected browser wait: %d", state)
		}
	}
}

//go:build windows

package startupprobe

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestBrowserExitFailurePreservesVerifiedIdentityAndPhaseTiming(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	browser := exec.Command(exe, "--browser-probe-fixture")
	browser.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = browser.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = browser.Process.Kill(); _ = browser.Wait() })
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(browser.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	var created, exited, kernel, user windows.Filetime
	err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	stamp := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	t.Setenv("WINONE_PROBE_BROWSER_PID", strconv.Itoa(browser.Process.Pid))
	t.Setenv("WINONE_PROBE_BROWSER_CREATED", strconv.FormatUint(stamp, 10))
	scratch := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = CheckDesktop(ctx, exe, scratch); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected exit observation timeout, got %v", err)
	}
	files, err := filepath.Glob(filepath.Join(scratch, "desktop-probe-failure-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("missing failure diagnostics", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Stage          string `json:"stage"`
		BrowserPID     uint32 `json:"browserProcessId"`
		BrowserCreated uint64 `json:"browserCreated"`
		UIElapsed      int64  `json:"uiElapsedMs"`
		WaitElapsed    int64  `json:"browserWaitMs"`
		Remaining      int64  `json:"remainingBeforeBrowserWaitMs"`
	}
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Stage != "browser-exit" || report.BrowserPID != uint32(browser.Process.Pid) || report.BrowserCreated != stamp || report.WaitElapsed <= 0 || report.Remaining <= 0 || report.UIElapsed < 0 {
		t.Fatalf("failure cannot be correlated to the owned process: %+v", report)
	}
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if _, present := fields["nonce"]; present {
		t.Fatal("diagnostic retained the private handshake nonce")
	}
}

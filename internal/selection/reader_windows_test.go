//go:build windows

package selection

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestReaderFixture(t *testing.T) {
	mode := os.Getenv("WINONE_SELECTION_READER_FIXTURE")
	if mode == "" {
		return
	}
	if mode == "child" {
		for {
			time.Sleep(time.Second)
		}
	}
	_, _ = io.ReadAll(os.Stdin)
	if mode == "valid" {
		_, _ = os.Stdout.WriteString(`{"status":"selected","text":"fixture selection","rectangles":[]}`)
		os.Exit(0)
	}
	if mode == "invalid" {
		_, _ = os.Stdout.WriteString(`{"status":"sensitive","text":"must not escape"}`)
		os.Exit(0)
	}
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run=^TestReaderFixture$")
	child.Env = append(os.Environ(), "WINONE_SELECTION_READER_FIXTURE=child")
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if child.Start() != nil {
		os.Exit(2)
	}
	data, _ := json.Marshal([]int{os.Getpid(), child.Process.Pid})
	if os.WriteFile(os.Getenv("WINONE_SELECTION_PID_FILE"), data, 0600) != nil {
		os.Exit(3)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestReaderRejectsSensitivePayloadAndTerminatesHungProcessTree(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	event := Event{Keyboard: true, Window: 1, ProcessID: 1}
	t.Setenv("WINONE_SELECTION_READER_FIXTURE", "valid")
	got, err := ReadSelection(context.Background(), self, event)
	if err != nil || got.Text != "fixture selection" {
		t.Fatal(got, err)
	}
	t.Setenv("WINONE_SELECTION_READER_FIXTURE", "invalid")
	if _, err = ReadSelection(context.Background(), self, event); err == nil {
		t.Fatal("sensitive payload accepted")
	}
	t.Setenv("WINONE_SELECTION_READER_FIXTURE", "hung")
	pidFile := filepath.Join(t.TempDir(), "pids.json")
	t.Setenv("WINONE_SELECTION_PID_FILE", pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	if _, err = ReadSelection(ctx, self, event); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	// The fixed product deadline can expire before a cold test binary reaches
	// the provider. Verify process-tree cleanup separately: wait until the
	// fixture publishes its pids, then cancel and require both to exit.
	cancelCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, readErr := readSelection(cancelCtx, self, event, 10*time.Second)
		done <- readErr
	}()
	var data []byte
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err = os.ReadFile(pidFile)
		if err == nil || !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		stop()
		<-done
		t.Fatal("fixture did not reach external provider", err)
	}
	stop()
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var pids []int
	if json.Unmarshal(data, &pids) != nil || len(pids) != 2 {
		t.Fatal("invalid process fixture")
	}
	for _, pid := range pids {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			continue
		}
		status, err := windows.WaitForSingleObject(handle, 2000)
		windows.CloseHandle(handle)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatal("reader child survived cancellation", strconv.Itoa(pid), err)
		}
	}
}
func TestWatcherCanStartStopAndRestart(t *testing.T) {
	if os.Getenv("WINONE_SELECTION_HOOK_TEST") != "1" {
		t.Skip("opt in to real global hook lifecycle")
	}
	for range 3 {
		watcher, err := StartWatcher()
		if err != nil {
			t.Fatal(err)
		}
		if err = watcher.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if activeWatcher.Load() != nil {
		t.Fatal("watcher remained active")
	}
}

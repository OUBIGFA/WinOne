//go:build windows

package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryImportProcess(t *testing.T) {
	path := os.Getenv("WINONE_HISTORY_IMPORT_TEST")
	if path == "" {
		return
	}
	program := filepath.Join(filepath.Dir(path), "program.exe")
	raw, _ := json.Marshal(startupHistoryData{Version: 1, Apps: map[string][]time.Time{program: {time.Now().UTC()}}})
	fmt.Println("ready")
	if err := NewStartupHistory(path).MergeImport(raw); err != nil {
		t.Fatal(err)
	}
}

func TestImportWaitsForAnotherProcessLaunchRefund(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup-history.json")
	history := NewStartupHistory(path)
	entry := ManagedAppEntry{ExePath: filepath.Join(filepath.Dir(path), "program.exe"), RunOnStartup: true, Schedule: Schedule{Enabled: true, FrequencyEnabled: true, FrequencyDays: 1, FrequencyRuns: 1}}
	entered, release := make(chan struct{}), make(chan struct{})
	launchDone := make(chan error, 1)
	go func() {
		_, err := history.Launch(entry, time.Now(), func() error { close(entered); <-release; return errors.New("launch failed") })
		launchDone <- err
	}()
	<-entered
	released := false
	defer func() {
		if !released {
			close(release)
			<-launchDone
		}
	}()
	command := exec.Command(os.Args[0], "-test.run=^TestHistoryImportProcess$")
	command.Env = append(os.Environ(), "WINONE_HISTORY_IMPORT_TEST="+path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			<-done
		}
	}()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("child did not reach merge")
	}
	select {
	case err := <-done:
		finished = true
		t.Fatalf("import bypassed active launch: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	released = true
	if err = <-launchDone; err == nil {
		t.Fatal("launch failure missing")
	}
	err = <-done
	finished = true
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := history.Launch(entry, time.Now(), func() error { t.Error("imported launch was refunded"); return nil })
	if err != nil || allowed {
		t.Fatalf("imported limit lost: %t %v", allowed, err)
	}
}

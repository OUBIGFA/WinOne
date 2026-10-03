//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeWarningsDoNotHideIndependentFailures(t *testing.T) {
	if warning := hostRuntimeWarnings("", "", nil, "en-US"); warning != "" {
		t.Fatal(warning)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		warning := hostRuntimeWarnings("restart failure", "permission failure", errors.New("log denied"), locale)
		if !strings.Contains(warning, "restart failure") || !strings.Contains(warning, "permission failure") || !strings.Contains(warning, "log denied") || strings.Count(warning, "\n") != 2 {
			t.Fatal(warning)
		}
	}
}

func TestHostLogIsBoundedAndRecordsEvents(t *testing.T) {
	directory := t.TempDir()
	if err := recordHostEvent(directory, "started", nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "logs", "host.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recordHostEvent(directory, "stopped", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 || !strings.Contains(string(data), "stopped") {
		t.Fatal("log did not reset at its limit", err)
	}
}

func TestConcurrentHostEventsRemainComplete(t *testing.T) {
	directory := t.TempDir()
	const count = 32
	start := make(chan struct{})
	failures := make(chan error, count)
	var group sync.WaitGroup
	for index := 0; index < count; index++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			failures <- recordHostEvent(directory, "tray-action-failed", fmt.Errorf("action %d", i))
		}(index)
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "logs", "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != count {
		t.Fatalf("recorded %d of %d events", len(lines), count)
	}
	seen := map[string]bool{}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("corrupted event: %s", line)
		}
		var event struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		seen[event.Error] = true
	}
	if len(seen) != count {
		t.Fatal("duplicate or missing event details")
	}
}

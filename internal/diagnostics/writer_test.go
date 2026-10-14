package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestModuleOutputUsesTheSingleWinOneLog(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "modules", "capture")
	writer, err := NewModuleWriter(module, "capture")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"first ", "message\nsecond"} {
		if _, err := writer.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(dir, "logs", "host.log"))), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d records", len(lines))
	}
	for i, want := range []string{"first message", "second"} {
		var event Event
		if err := json.Unmarshal([]byte(lines[i]), &event); err != nil {
			t.Fatal(err)
		}
		if event.Source != "capture" || event.Message != want {
			t.Fatalf("record: %+v", event)
		}
	}
	if _, err := os.Stat(filepath.Join(module, "worker.log")); !os.IsNotExist(err) {
		t.Fatal("independent worker log exists", err)
	}
}

func TestUnifiedLogSerializesConcurrentSources(t *testing.T) {
	dir := t.TempDir()
	var wait sync.WaitGroup
	for _, source := range []string{"host", "capture", "wintray", "always-on-top"} {
		wait.Add(1)
		go func(source string) {
			defer wait.Done()
			for i := 0; i < 40; i++ {
				if err := Record(dir, Event{Source: source, Event: "test", Message: "complete record"}); err != nil {
					t.Error(err)
					return
				}
			}
		}(source)
	}
	wait.Wait()
	lines := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(dir, "logs", "host.log"))), "\n")
	if len(lines) != 160 {
		t.Fatalf("lost records: %d", len(lines))
	}
	for _, line := range lines {
		var event Event
		if json.Unmarshal([]byte(line), &event) != nil {
			t.Fatal("torn record")
		}
	}
}

func TestUnifiedLogBoundsLongOutputAndKeepsValidUTF8(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewModuleWriter(filepath.Join(dir, "modules", "selection"), "selection")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write([]byte(strings.Repeat("文字", 20000))); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(readFile(t, filepath.Join(dir, "logs", "host.log"))), "\n") {
		var event Event
		if json.Unmarshal([]byte(line), &event) != nil || len(event.Message) > 8192 {
			t.Fatal("invalid or unbounded output record")
		}
	}
}

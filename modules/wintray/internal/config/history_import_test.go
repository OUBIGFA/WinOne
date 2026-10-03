package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryImportPreservesFrequencyAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	history := NewStartupHistory(filepath.Join(dir, "startup-history.json"))
	path := filepath.Join(dir, "example.exe")
	now := time.Now().UTC()
	data, _ := json.Marshal(startupHistoryData{Version: 1, Apps: map[string][]time.Time{path: {now.Add(-time.Hour), now.Add(-time.Hour)}}})
	for range 2 {
		if err := history.MergeImport(data); err != nil {
			t.Fatal(err)
		}
	}
	entry := ManagedAppEntry{ExePath: path, RunOnStartup: true, Schedule: Schedule{Enabled: true, FrequencyEnabled: true, FrequencyDays: 1, FrequencyRuns: 2}}
	launched := false
	allowed, err := history.Launch(entry, now, func() error { launched = true; return nil })
	if err != nil || allowed || launched {
		t.Fatalf("limit reset: %t %t %v", allowed, launched, err)
	}
	stored, _ := os.ReadFile(history.path)
	records, err := DecodeHistory(stored)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, stamps := range records {
		total += len(stamps)
	}
	if total != 2 {
		t.Fatalf("duplicate import inflated count: %d", total)
	}
	before := string(stored)
	if err = history.MergeImport([]byte(`{"version":8,"apps":{}}`)); err == nil {
		t.Fatal("invalid import accepted")
	}
	after, _ := os.ReadFile(history.path)
	if string(after) != before {
		t.Fatal("invalid history altered destination")
	}
}

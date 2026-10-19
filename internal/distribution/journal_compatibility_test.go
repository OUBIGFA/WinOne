package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Dependency filenames change between releases. Reading durable history must
// not apply today's packaging allowlist to yesterday's completed transaction.
func TestUpgradeJournalRetainsHistoricalDependencyNames(t *testing.T) {
	for _, phase := range []string{"prepared", "committed", "rolled-back"} {
		t.Run(phase, func(t *testing.T) {
			plan := journalPlan()
			plan.Files[0].Path = "capture/Ocr/python/Lib/site-packages/retired_dependency-1.0.dist-info/METADATA"
			plan.Files[0].PreviousPath = plan.Files[0].Path
			if allowed(plan.Files[0].Path) { t.Fatal("fixture must be outside the current payload inventory") }
			path := filepath.Join(t.TempDir(), "journal.json")
			store, err := CreateUpgradeJournal(path, plan)
			if err != nil { t.Fatal(err) }
			state := store.Snapshot()
			state.Phase = phase
			if phase == "committed" {
				state.FileStates[0] = "applied"
				for name := range state.MetadataStates { state.MetadataStates[name] = "applied" }
			}
			raw, err := json.Marshal(state)
			if err != nil { t.Fatal(err) }
			if err = os.WriteFile(path, raw, 0600); err != nil { t.Fatal(err) }
			reopened, err := OpenUpgradeJournal(path)
			if err != nil || reopened.Snapshot().Phase != phase { t.Fatalf("historical journal: %v", err) }
		})
	}
}

func TestUpgradeJournalHistoricalNamesStillRejectUnsafePaths(t *testing.T) {
	for _, name := range []string{"notes.txt", "profiles/settings.json", "../WinOne.exe", "capture/../../outside", `C:\WinOne.exe`, `capture\file.dll`, "capture/file.dll:stream", "capture/NUL.txt", "capture/file. ", "/WinOne.exe"} {
		t.Run(name, func(t *testing.T) {
			plan := journalPlan()
			plan.Files[0].Path, plan.Files[0].PreviousPath = name, name
			if _, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "journal.json"), plan); err == nil { t.Fatal("unsafe historical path accepted") }
		})
	}
	plan := journalPlan()
	duplicate := plan.Files[0]
	duplicate.Path, duplicate.PreviousPath = "winone.EXE", "winone.EXE"
	plan.Files = append(plan.Files, duplicate)
	if _, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "journal.json"), plan); err == nil { t.Fatal("case alias accepted") }
}

package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The last launch is written in the provider worker and read in the shared
// process. Exercise that persisted boundary without starting Claude itself.
func TestWinOneClaudeLaunchStatusPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last.json")
	t.Setenv("WINONE_AI_CLAUDE_RUN_FILE", path)
	run := ClaudeLastRun{At: time.Now()}
	winOneRememberClaudeRun(run)
	mode, err := WinOneClaudeMode()
	if err != nil || mode.Last == nil || !mode.Last.At.Equal(run.At) {
		t.Fatalf("status lost: %+v %v", mode, err)
	}
	if err = os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = WinOneClaudeMode(); err == nil {
		t.Fatal("invalid launch status hidden")
	}
}

package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ageFile forces a file's mtime to simulate when it was last written.
func ageFile(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestPruneKeepsRecentHostEvents(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	expired := now.Add(-72 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	writeFile(t, filepath.Join(dir, "logs", "host.log"),
		`{"time":"`+expired+`","event":"started"}`+"\n"+
			`{"time":"`+recent+`","event":"stopped"}`+"\n")

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	want := `{"time":"` + recent + `","event":"stopped"}` + "\n"
	if got := readFile(t, filepath.Join(dir, "logs", "host.log")); got != want {
		t.Fatalf("host.log = %q, want %q", got, want)
	}
}

// Unparseable trailing lines belong to the preceding entry: a recent event
// keeps its corrupted tail, an expired event drops it.
func TestPruneKeepsGarbageWithRecentHostEvent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	expired := now.Add(-72 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	writeFile(t, filepath.Join(dir, "logs", "host.log"),
		`{"time":"`+expired+`","event":"started"}`+"\n"+"corrupt line\n"+
			`{"time":"`+recent+`","event":"stopped"}`+"\n"+"trailing junk\n")

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	want := `{"time":"` + recent + `","event":"stopped"}` + "\n" + "trailing junk\n"
	if got := readFile(t, filepath.Join(dir, "logs", "host.log")); got != want {
		t.Fatalf("host.log = %q, want %q", got, want)
	}
}

func TestPruneKeepsMultiLineWinTrayEntries(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	expired := now.Add(-72 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	writeFile(t, filepath.Join(dir, "modules", "wintray", "wintray.log"),
		expired+" [INFO] old first line\ncontinuation of old\n"+
			recent+" [WARN] recent entry\nrecent continuation\n")

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	want := recent + " [WARN] recent entry\nrecent continuation\n"
	if got := readFile(t, filepath.Join(dir, "modules", "wintray", "wintray.log")); got != want {
		t.Fatalf("wintray.log = %q, want %q", got, want)
	}
}

func TestPruneEmptiesFullyExpiredWinTrayLog(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	expired := now.Add(-72 * time.Hour).Format(time.RFC3339)
	path := filepath.Join(dir, "modules", "wintray", "wintray.log")
	writeFile(t, path, expired+" [INFO] only old content\n")

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if got := readFile(t, path); got != "" {
		t.Fatalf("wintray.log = %q, want empty", got)
	}
}

// A log with nothing expired must keep its exact bytes, including a missing
// trailing newline.
func TestPruneKeepsFreshWinTrayLogByteForByte(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	content := recent + " [WARN] no trailing newline"
	path := filepath.Join(dir, "modules", "wintray", "wintray.log")
	writeFile(t, path, content)

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if got := readFile(t, path); got != content {
		t.Fatalf("wintray.log = %q, want %q", got, content)
	}
}

func TestPruneRemovesStaleRotatedLog(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	stale := filepath.Join(dir, "modules", "wintray", "wintray.log.old")
	fresh := filepath.Join(dir, "modules", "wintray", "activity.log.old")
	writeFile(t, stale, "rotated content")
	writeFile(t, fresh, "rotated content")
	ageFile(t, stale, now.Add(-72*time.Hour))
	ageFile(t, fresh, now.Add(-1*time.Hour))

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale rotated log still exists: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh rotated log removed: %v", err)
	}
}

func TestPruneKeepsUnrelatedOldBackups(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	path := filepath.Join(dir, "modules", "wintray", "settings.json.old")
	writeFile(t, path, "configuration backup")
	ageFile(t, path, now.Add(-72*time.Hour))
	if err := Prune(dir, now); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "configuration backup" {
		t.Fatalf("backup changed: %q", got)
	}
}

func TestPruneTruncatesStaleWorkerLog(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	stale := filepath.Join(dir, "modules", "capture", "worker.log")
	fresh := filepath.Join(dir, "modules", "wintray", "worker.log")
	writeFile(t, stale, "stale worker output")
	writeFile(t, fresh, "fresh worker output")
	ageFile(t, stale, now.Add(-72*time.Hour))
	ageFile(t, fresh, now.Add(-1*time.Hour))

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if got := readFile(t, stale); got != "" {
		t.Fatalf("stale worker.log = %q, want empty", got)
	}
	if got := readFile(t, fresh); got != "fresh worker output" {
		t.Fatalf("fresh worker.log = %q, want original content", got)
	}
}

func TestPruneRemovesStaleNativeLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	base := filepath.Join(dir, "modules", "always-on-top", "native")
	stale := filepath.Join(base, "native_2026-10-01.log")
	fresh := filepath.Join(base, "native_2026-10-03.log")
	other := filepath.Join(base, "native_2026-10-01.txt")
	for _, path := range []string{stale, other} {
		writeFile(t, path, "engine output")
		ageFile(t, path, now.Add(-72*time.Hour))
	}
	writeFile(t, fresh, "engine output")
	ageFile(t, fresh, now.Add(-1*time.Hour))

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale native log still exists: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh native log removed: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-log file removed: %v", err)
	}
}

func TestPruneToleratesMissingDirectory(t *testing.T) {
	if err := Prune(filepath.Join(t.TempDir(), "absent"), time.Now()); err != nil {
		t.Fatalf("Prune on missing directory: %v", err)
	}
	if err := Prune("", time.Now()); err != nil {
		t.Fatalf("Prune with empty directory: %v", err)
	}
}

// The retention window is inclusive: an entry exactly 48 hours old is kept,
// one second older is dropped.
func TestPruneRetentionBoundary(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.Local)
	atCutoff := now.Add(-retentionWindow).Format(time.RFC3339)
	justBefore := now.Add(-retentionWindow - time.Second).Format(time.RFC3339)
	writeFile(t, filepath.Join(dir, "logs", "host.log"),
		`{"time":"`+justBefore+`","event":"expired"}`+"\n"+
			`{"time":"`+atCutoff+`","event":"boundary"}`+"\n")

	if err := Prune(dir, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	want := `{"time":"` + atCutoff + `","event":"boundary"}` + "\n"
	if got := readFile(t, filepath.Join(dir, "logs", "host.log")); got != want {
		t.Fatalf("host.log = %q, want %q", got, want)
	}
}

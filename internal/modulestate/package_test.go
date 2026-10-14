package modulestate

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/settings"
)

func TestPackageBindingStartsDisabledAndSurvivesLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	state, err := store.BindPackage("example.tool", digest)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Modules["example.tool"]
	if record.Enabled || record.PendingStop || record.PackageSHA256 != digest {
		t.Fatal(record)
	}
	state, err = store.SetIntent(state.Revision, "example.tool", true)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.BindPackage("example.tool", digest)
	if err != nil || !replay.Modules["example.tool"].Enabled || replay.Revision != state.Revision {
		t.Fatal("replay changed enabled intent", replay, err)
	}
	state, err = store.SetIntent(state.Revision, "example.tool", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindPackage("example.tool", digest); err == nil {
		t.Fatal("pending stop was bypassed")
	}
	state, err = store.FinishStop("example.tool")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().Modules["example.tool"].PackageSHA256 != digest {
		t.Fatal("binding lost on disable or restart")
	}
}

func TestPackageChangeBindingIsInactiveComparedAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old, next, other := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	if _, err = store.BindPackage("example.tool", old); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReplacePackageBinding("example.tool", other, next); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("stale binding accepted", err)
	}
	saved, err := store.ReplacePackageBinding("example.tool", old, next)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ReplacePackageBinding("example.tool", old, next)
	if err != nil || replayed.Revision != saved.Revision {
		t.Fatal("replayed binding changed revision", err)
	}
	if _, err = store.SetIntent(saved.Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReplacePackageBinding("example.tool", next, ""); err == nil {
		t.Fatal("active binding was cleared")
	}
	if _, err = store.SetIntent(store.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReplacePackageBinding("example.tool", next, ""); err == nil {
		t.Fatal("pending recovery was cleared")
	}
	if _, err = store.FinishStop("example.tool"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReplacePackageBinding("example.tool", next, ""); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if record := reopened.Snapshot().Modules["example.tool"]; record.PackageSHA256 != "" || record.Enabled || record.PendingStop {
		t.Fatal(record)
	}
}

func TestBindingCannotReplaceAnActivePackageOrBuiltin(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	state, err := store.BindPackage("example.tool", digest)
	if err != nil {
		t.Fatal(err)
	}
	state, err = store.SetIntent(state.Revision, "example.tool", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindPackage("example.tool", strings.Repeat("b", 64)); err == nil {
		t.Fatal("active package replaced")
	}
	if _, err = store.BindPackage("capture", digest); err == nil {
		t.Fatal("builtin rebound as package")
	}
	if _, err = store.BindPackage("another.tool", "invalid"); err == nil {
		t.Fatal("invalid binding accepted")
	}
	if store.Snapshot().Revision != state.Revision {
		t.Fatal("rejection changed module intent")
	}
}

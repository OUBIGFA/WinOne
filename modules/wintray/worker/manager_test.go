package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"winone/modules/wintray/internal/config"
)

type crashEffects struct{}

func (crashEffects) Reconcile(config.Settings, bool) error { os.Exit(77); return nil }

func TestInterruptedSaveRequiresRecoveryAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := config.NewStore(path)
	before := config.DefaultSettings()
	before.RunAtLogon = false
	if err := store.Save(before); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSaveCrashHelper$")
	cmd.Env = append(os.Environ(), "WINONE_CRASH_SETTINGS="+path)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("crash helper: %v", err)
	}
	disk, err := store.LoadWithError()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(store, disk, &effects{})
	if m.Snapshot().RecoveryError == "" {
		t.Fatal("interrupted save was silently accepted")
	}
	if _, err = m.Save(m.Snapshot().Revision, before); err == nil {
		t.Fatal("edited before recovery")
	}
	if _, err = m.Recover(true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.LoadWithError()
	if err != nil || reloaded.StartupIntervalSeconds != 3 {
		t.Fatalf("recovered %+v, %v", reloaded, err)
	}
	if m.Snapshot().RecoveryError != "" {
		t.Fatal("successful recovery remained blocked")
	}
}

func TestSaveCrashHelper(t *testing.T) {
	path := os.Getenv("WINONE_CRASH_SETTINGS")
	if path == "" {
		return
	}
	store := config.NewStore(path)
	initial, err := store.LoadWithError()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(store, initial, crashEffects{})
	next := initial
	next.StartupIntervalSeconds = 19
	_, _ = m.Save(m.Snapshot().Revision, next)
	t.Fatal("expected process exit during system change")
}

func TestCorruptRecoveryJournalBlocksWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := config.NewStore(path)
	initial := config.DefaultSettings()
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	journal := path + ".transaction.json"
	if err := os.WriteFile(journal, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager(store, initial, &effects{})
	if m.Snapshot().RecoveryError == "" {
		t.Fatal("corrupt recovery journal ignored")
	}
	if _, err := m.Recover(true); err == nil {
		t.Fatal("corrupt journal was treated as recovered")
	}
	data, _ := os.ReadFile(journal)
	if string(data) != "damaged" {
		t.Fatal("corrupt evidence overwritten")
	}
}

func TestFirstSaveRecoversWhenSettingsFileWasNotCreated(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	before := config.DefaultSettings()
	before.RunAtLogon = false
	after := before
	after.StartupIntervalSeconds = 19
	m := NewManager(store, before, &effects{})
	if err := m.writeTransaction(transaction{Version: 1, Before: before, After: after, BeforeExisted: false}); err != nil {
		t.Fatal(err)
	}
	restarted := NewManager(store, before, &effects{})
	if _, err := restarted.Recover(true); err != nil {
		t.Fatal(err)
	}
	disk, err := store.LoadWithError()
	if err != nil || disk.RunAtLogon || disk.StartupIntervalSeconds != 3 {
		t.Fatalf("recovered %+v, %v", disk, err)
	}
}

type effects struct {
	fail    bool
	applied config.Settings
}

func (e *effects) Reconcile(s config.Settings, allow bool) error {
	if e.fail && len(s.ManagedApps) > 0 {
		return errors.New("task update refused")
	}
	e.applied = s
	return nil
}

func TestSaveRestoresSettingsWhenSystemChangeFails(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	initial := config.DefaultSettings()
	initial.RunAtLogon = false
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	effect := &effects{fail: true}
	m := NewManager(store, initial, effect)
	before := m.Snapshot()
	next := before.Settings
	next.ManagedApps = []config.ManagedAppEntry{{ID: "a", Name: "A", ExePath: `C:\Apps\A.exe`}}
	if _, err := m.Save(before.Revision, next); err == nil {
		t.Fatal("system failure accepted")
	}
	onDisk, err := store.LoadWithError()
	if err != nil || len(onDisk.ManagedApps) != 0 {
		t.Fatalf("rollback failed: %+v %v", onDisk, err)
	}
	if m.Snapshot().Revision != before.Revision {
		t.Fatal("failed save changed revision")
	}
}

func TestSaveRejectsStaleRevisionAndDuplicateIDs(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	m := NewManager(store, config.DefaultSettings(), &effects{})
	before := m.Snapshot()
	next := before.Settings
	next.StartupIntervalSeconds = 5
	if _, err := m.Save(before.Revision, next); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(before.Revision, before.Settings); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	current := m.Snapshot()
	next = current.Settings
	next.ManagedApps = []config.ManagedAppEntry{{ID: "a", Name: "A", ExePath: `C:\A.exe`}, {ID: "a", Name: "B", ExePath: `C:\B.exe`}}
	if _, err := m.Save(current.Revision, next); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}

func TestValidationPreservesScriptRestrictions(t *testing.T) {
	s := config.DefaultSettings()
	s.ManagedApps = []config.ManagedAppEntry{{ID: "a", Name: "script", ExePath: `C:\job.ps1`, LaunchViaLogonTask: true}}
	if err := Validate(s); err == nil {
		t.Fatal("script accepted task launch mode")
	}
}

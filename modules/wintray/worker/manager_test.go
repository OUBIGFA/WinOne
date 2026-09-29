package worker

import (
	"errors"
	"path/filepath"
	"testing"
	"winone/modules/wintray/internal/config"
)

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

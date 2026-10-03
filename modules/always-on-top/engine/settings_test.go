package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyFlagDoesNotControlEngineOrRewriteOnRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := Defaults()
	legacy.Enabled = false
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Snapshot().Enabled {
		t.Fatal("legacy flag remained a second engine switch")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("reading preferences rewrote legacy migration input")
	}
}

func TestManagedSettingsCannotBeWrittenByWorker(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "settings.json")
	legacy := Defaults()
	legacy.FrameThickness = 4
	legacyData, _ := json.Marshal(legacy)
	if err := os.WriteFile(legacyPath, legacyData, 0600); err != nil {
		t.Fatal(err)
	}
	managed := Defaults()
	managed.Revision = 12
	managed.FrameThickness = 8
	managedData, _ := json.Marshal(managed)
	if err := os.WriteFile(filepath.Join(directory, "managed-settings.json"), managedData, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSettings(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsManaged() || store.Snapshot().FrameThickness != 8 || store.Snapshot().Revision != 12 {
		t.Fatal(store.Snapshot())
	}
	next := store.Snapshot()
	next.FrameThickness = 10
	if _, err = store.Save(next); err == nil {
		t.Fatal("worker overwrote Host preferences")
	}
	after, _ := os.ReadFile(legacyPath)
	if string(after) != string(legacyData) {
		t.Fatal("legacy source overwritten")
	}
}

func TestManagedReloadRestoresRuntimeOnFailure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "managed-settings.json")
	before := Defaults()
	data, _ := json.Marshal(before)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSettings(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	next := before
	next.Revision = 5
	next.FrameThickness = 9
	data, _ = json.Marshal(next)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	runtimeThickness := before.FrameThickness
	_, err = store.ReloadManaged(func(value Settings) error {
		runtimeThickness = value.FrameThickness
		if value.FrameThickness == 9 {
			return errors.New("native apply failed")
		}
		return nil
	})
	if err == nil || runtimeThickness != before.FrameThickness || store.Snapshot().Revision != 0 {
		t.Fatal(runtimeThickness, store.Snapshot(), err)
	}
	actual, err := store.ReloadManaged(func(value Settings) error { runtimeThickness = value.FrameThickness; return nil })
	if err != nil || actual.Revision != 5 || runtimeThickness != 9 {
		t.Fatal(actual, err)
	}
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReloadManaged(func(value Settings) error { runtimeThickness = 99; return nil }); err == nil || runtimeThickness != 9 {
		t.Fatal("corrupt projection reached native apply")
	}
}

func TestSettingsPersistAndRejectStaleRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := OpenSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	next := before
	next.FrameThickness = 7
	if _, err = s.Save(next); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(before); err == nil {
		t.Fatal("stale settings accepted")
	}
	reopened, err := OpenSettings(path)
	if err != nil || reopened.Snapshot().FrameThickness != 7 {
		t.Fatalf("reload: %v", err)
	}
}

func TestSettingsRejectInvalidColorWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, _ := OpenSettings(path)
	next := s.Snapshot()
	next.FrameColor = "not-a-color"
	if _, err := s.Save(next); err == nil {
		t.Fatal("invalid colour accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid settings written")
	}
}

func TestColorUsesWindowsColorRefOrder(t *testing.T) {
	s := Defaults()
	s.FrameColor = "#123456"
	if got := s.ColorRef(); got != 0x563412 {
		t.Fatalf("COLORREF=%x", got)
	}
}

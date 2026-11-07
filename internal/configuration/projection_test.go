package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"winone/internal/settings"
	"winone/modules/wintray/preferences"
)

// Real durable projections, not sleep-based latency substitutes. The native
// effects behind a projection are checked separately by the Host integration.
func projectionFixture(t testing.TB) (*HostStore, map[string]int, string) {
	t.Helper()
	root := t.TempDir()
	calls := map[string]int{}
	adapters := []ModuleAdapter{}
	for _, id := range []string{"wintray", "capture", "always-on-top", "selection", "quick-assistant", "stroke", "timed-power"} {
		adapters = append(adapters, ModuleAdapter{ID: id,
			Load:     func() (json.RawMessage, error) { return json.RawMessage(`{"value":1}`), nil },
			Validate: func(raw json.RawMessage) error { var value map[string]any; return json.Unmarshal(raw, &value) },
			Apply: func(raw json.RawMessage, _ uint64) error {
				calls[id]++
				if err := settings.WriteAtomic(filepath.Join(root, id+"-projection.json"), raw); err != nil {
					return err
				}
				if string(raw) == `{"value":9}` {
					return errors.New("native projection refused after writing")
				}
				return nil
			},
		})
	}
	store, err := OpenHost(root, adapters...)
	if err != nil {
		t.Fatal(err)
	}
	clear(calls)
	return store, calls, root
}

func TestModuleProjectionOnlyWritesChangedPreferencesAndKeepsTheGlobalGuard(t *testing.T) {
	store, calls, root := projectionFixture(t)
	global := 0
	store.SetApply(func(settings.Settings) error { global++; return nil })
	_, revision, err := store.Module("wintray")
	if err != nil {
		t.Fatal(err)
	}
	_, next, err := store.SaveModule("wintray", revision, json.RawMessage(`{"value":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if calls["wintray"] != 1 || global != 1 || next != revision+1 {
		t.Fatalf("changed projection or global guard: calls=%v global=%d revision=%d", calls, global, next)
	}
	for id, count := range calls {
		if id != "wintray" && count != 0 {
			t.Errorf("unrelated %s projection was rewritten %d times", id, count)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "wintray-projection.json"))
	if err != nil || string(data) != `{"value":2}` {
		t.Fatalf("requested settings were not durable: %s %v", data, err)
	}
	// Restart is a full projection, not a persistent cache which could bless
	// missing worker files or native state left by an interrupted transaction.
	before := calls["wintray"]
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	for id, count := range calls {
		want := 1
		if id == "wintray" {
			want = before + 1
		}
		if count != want {
			t.Errorf("explicit recovery skipped %s: %d != %d", id, count, want)
		}
	}
}

func TestModuleProjectionFailureStillCompensatesEveryNativeNamespace(t *testing.T) {
	store, calls, root := projectionFixture(t)
	_, revision, _ := store.Module("wintray")
	if _, _, err := store.SaveModule("wintray", revision, json.RawMessage(`{"value":9}`)); err == nil {
		t.Fatal("failed projection was accepted")
	}
	if store.CommittedDocument().Revision != revision || store.RecoveryError() != nil {
		t.Fatal("failed save changed revision or lost recovery state")
	}
	for _, id := range []string{"wintray", "capture", "always-on-top", "selection", "quick-assistant", "stroke", "timed-power"} {
		want := 1
		if id == "wintray" {
			want = 2
		}
		if calls[id] != want {
			t.Errorf("compensation did not reconcile %s: %d != %d", id, calls[id], want)
		}
		data, err := os.ReadFile(filepath.Join(root, id+"-projection.json"))
		if err != nil || string(data) != `{"value":1}` {
			t.Errorf("%s retained failed settings: %s %v", id, data, err)
		}
	}
}

func TestModuleProjectionRefreshesAnUnchangedNamespaceWhenItsEnablementChanges(t *testing.T) {
	count := 0
	projected := false
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"value":1}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { count++; projected = true; return nil }}
	lifecycle := ModuleLifecycle{Initial: map[string]bool{"example.tool": false}, Before: func(map[string]bool) error { return nil }, After: func(target map[string]bool) error {
		if target["example.tool"] && !projected {
			return errors.New("enabled before projection")
		}
		return nil
	}}
	store, err := OpenHostWithLifecycle(t.TempDir(), lifecycle, adapter)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	projected = false
	next := store.CommittedDocument()
	next.ModuleEnabled[adapter.ID] = true
	if _, err = store.CommitDocument(next); err != nil || count != 1 {
		t.Fatalf("changed enablement skipped its current projection: %d %v", count, err)
	}
}

func TestModuleProjectionIncludesGlobalCompatibilityChangesPreparedForWorkers(t *testing.T) {
	calls := 0
	adapter := ModuleAdapter{ID: "example.tool", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"language":"zh-CN"}`), nil }, Validate: func(raw json.RawMessage) error { var value map[string]any; return json.Unmarshal(raw, &value) }, Prepare: func(raw json.RawMessage, global settings.Settings) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"language": global.Language})
	}, Apply: func(json.RawMessage, uint64) error { calls++; return nil }}
	store, err := OpenHost(t.TempDir(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	next := store.Snapshot()
	next.Language = "en-US"
	if _, err = store.Save(next); err != nil || calls != 1 {
		t.Fatalf("global language did not reach worker: calls=%d err=%v", calls, err)
	}
	raw, _, _ := store.Module(adapter.ID)
	if string(raw) != `{"language":"en-US"}` {
		t.Fatal(string(raw))
	}
}

// Same 100-program document and seven fsynced projection files before/after.
// This measures the canonical Host save/serialization/durable-write path, not
// physical click-to-photon latency or real Windows startup reconciliation.
func BenchmarkConfigurationSave100Programs7Projections(b *testing.B) {
	store, _, root := projectionFixture(b)
	programs := make([]map[string]any, 100)
	for i := range programs {
		programs[i] = map[string]any{"id": fmt.Sprintf("program-%d", i), "name": fmt.Sprintf("Program %d", i), "exePath": filepath.Join(root, fmt.Sprintf("fixture-%d.exe", i)), "runOnStartup": false, "trayBehavior": map[string]any{}, "schedule": map[string]any{"autoExitMinutes": 30, "frequencyDays": 1, "frequencyRuns": 1}}
	}
	value := preferences.Defaults()
	data, _ := json.Marshal(programs)
	if err := json.Unmarshal(data, &value.ManagedApps); err != nil {
		b.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		b.Fatal(err)
	}
	if _, err = preferences.Decode(raw); err != nil {
		b.Fatal(err)
	}
	_, revision, _ := store.Module("wintray")
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		value.ManagedApps[0].Name = fmt.Sprintf("Saved %d", i)
		raw, err := json.Marshal(value)
		if err != nil {
			b.Fatal(err)
		}
		_, revision, err = store.SaveModule("wintray", revision, raw)
		if err != nil {
			b.Fatal(err)
		}
	}
}

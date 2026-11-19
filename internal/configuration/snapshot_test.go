package configuration

import (
	"encoding/json"
	"strings"
	"testing"
)

// A large program list must not be copied by unrelated shortcut, theme and
// module-status reads. The fixture uses the public store and real persistence.
func BenchmarkPreferenceReadsWithLargeOtherModule(b *testing.B) {
	store, _, _ := projectionFixture(b)
	_, revision, err := store.Module("wintray")
	if err != nil {
		b.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"programData": strings.Repeat("x", 256<<10)})
	if _, _, err := store.SaveModule("wintray", revision, data); err != nil {
		b.Fatal(err)
	}
	b.Run("global", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = store.Snapshot()
		}
	})
	b.Run("other-module", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := store.Module("capture"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestPreferenceReadsAreDetachedAndCarryCommittedRevision(t *testing.T) {
	store, _, _ := projectionFixture(t)
	before := store.Snapshot()
	raw, revision, err := store.Module("capture")
	if err != nil || revision != before.Revision {
		t.Fatalf("read revision %d: %v", revision, err)
	}
	raw[0] = '!'
	before.Startup.RunAtLogon = !before.Startup.RunAtLogon
	current := store.Snapshot()
	again, _, err := store.Module("capture")
	if err != nil || string(again) != `{"value":1}` || current.Startup.RunAtLogon == before.Startup.RunAtLogon {
		t.Fatal("caller changed committed preferences", err)
	}
	_, next, err := store.SaveModule("capture", revision, json.RawMessage(`{"value":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if raw, got, err := store.Module("capture"); err != nil || got != next || string(raw) != `{"value":2}` || store.Snapshot().Revision != next {
		t.Fatalf("reads lost committed content/revision: %s %d %v", raw, got, err)
	}
}

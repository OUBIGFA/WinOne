package aiservices

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/internal/ai"
)

// A second catalog omits an old disabled model. Unmarshalling into the old
// metadata map used to retain that model and made the real Host reject Save.
func TestModelRefreshDropsRetiredMetadataAndSaves(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	pr.CatalogModels = []string{"gpt-a", "gpt-b"}
	pr.ModelMetadata = map[string]metadata{"gpt-b": {Value: modelMetadata{ContextWindow: ptr(32000.0)}}}
	panel := newModelsPanel(p, pr)
	panel.fetch(pr)
	tt := ui.NewTester(func(c *ui.Context) { panel.View(c, p.draft.Value.find("one")) }, 900, 1200)
	settle(h, tt)
	if panel.err != "" {
		t.Fatal(panel.err)
	}
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p.draft)
	if err != nil {
		t.Fatal(err)
	}
	var configuration ai.Configuration
	if err = json.Unmarshal(raw, &configuration); err != nil {
		t.Fatal(err)
	}
	configuration.Revision = store.Configuration().Revision
	configuration.Engine, configuration.OCREngine, configuration.Target, configuration.Prompt = "google", "local", "zh-CN", "Translate."
	if _, err = store.SaveConfiguration(configuration, nil); err != nil {
		t.Fatalf("refreshed catalog cannot save: %v", err)
	}
	if _, ok := p.draft.Value.find("one").ModelMetadata["gpt-b"]; ok {
		t.Fatal("retired model metadata survived catalog replacement")
	}
}

func TestModelListOrderIsStableAcrossCatalogOrderAndSave(t *testing.T) {
	_, _, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	pr.Models = []string{"z-model", "a-model"}
	pr.AvailableModels = ptr([]string{"z-model", "b-model", "a-model"})
	want := []string{"a-model", "b-model", "z-model"}
	before := pr.available()
	if !reflect.DeepEqual(before, want) {
		t.Fatalf("catalog order is unstable: %v", before)
	}
	next := clone(*p.draft)
	next.Value.Providers[0].Value.AvailableModels = ptr([]string{"b-model", "a-model", "z-model"})
	p.accept(next)
	if got := p.draft.Value.find("one").available(); !reflect.DeepEqual(got, before) {
		t.Fatalf("saved list changed order: %v -> %v", before, got)
	}
}

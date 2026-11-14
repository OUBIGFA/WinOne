package aiservices

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
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

// The native fetch/check/save actions cross the real Host store. Defaults
// must stay absent after saving and reopening, including an older catalog
// whose advertised output was larger than its window.
func TestNativeModelSelectionSavesAndRestoresTheModelDefault(t *testing.T) {
	f, h, p, tt := setup(t)
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := ai.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.Providers = []ai.Provider{{ID: "one", Name: "Relay", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{}}}
	saved, err := store.SaveConfiguration(configuration, nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	raw, _ := json.Marshal(saved)
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	p.accept(doc)
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "capture.ai.models.fetch":
			return map[string]any{"models": []ai.ModelOption{{ID: "vendor-model", Name: "Vendor model", Metadata: ai.ModelMetadata{ContextWindow: 128000, MaxTokens: 384000, Vision: new(true), Levels: []string{"light", "deep"}, Efforts: []string{"light", "deep"}}}}}, nil
		case "capture.ai.configuration.save":
			var input struct {
				Configuration ai.Configuration `json:"configuration"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, err
			}
			return store.SaveConfiguration(input.Configuration, nil)
		}
		return f.handle(method, params)
	})
	settle(h, tt)
	if err = tt.Click(i18n.T("ai.fetchModels")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err = tt.Click("Vendor model"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	h.Env.Rail.Save.Run()
	settle(h, tt)
	if p.err != "" || p.dirty() {
		t.Fatal("native selection could not save", p.err)
	}
	reopened, err := ai.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := reopened.Configuration().Provider("one")
	if len(provider.Models) != 1 || provider.Models[0] != "vendor-model" || !reflect.DeepEqual(provider.ModelMetadataFor("vendor-model"), ai.ModelMetadata{}) {
		t.Fatal("selecting a model installed overrides")
	}
	p.editor.models.editing = "vendor-model"
	settle(h, tt)
	if err = tt.Click("200K"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err = tt.Click(i18n.T("ai.modelRestoreDefaults")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if got := p.draft.Value.find("one").ModelMetadata["vendor-model"].Value; got.ContextWindow != nil || got.MaxTokens != nil || got.Vision != nil {
		t.Fatal("restore default retained a capability override")
	}
	if p.draft.Value.find("one").CatalogMetadata["vendor-model"].ContextWindow != 128000 {
		t.Fatal("restore default discarded the vendor report")
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

// Clearing the override must also clear a value typed into the real input.
// Otherwise restore shows the old value and can write it back on a later edit.
func TestNativeModelRestoreClearsTypedContextBeforeDrawing(t *testing.T) {
	_, h, p, tt := setup(t)
	p.editor.models.editing = "gpt-a"
	settle(h, tt)
	label := i18n.T("ai.contextWindow")
	field, ok := tt.AccessByRole(ui.RoleTextField, label)
	if !ok {
		t.Fatal("missing context input")
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Type("200000")
	settle(h, tt)
	if field, ok := tt.AccessByRole(ui.RoleTextField, label); !ok || field.Value != "200000" {
		t.Fatalf("typed context was not displayed: %+v", field)
	}
	if err := tt.Click(i18n.T("ai.modelRestoreDefaults")); err != nil {
		t.Fatal(err)
	}
	if field, ok := tt.AccessByRole(ui.RoleTextField, label); !ok || field.Value != "" {
		t.Fatalf("restore left the old context visible: %+v", field)
	}
	if got := p.draft.Value.find("one").ModelMetadata["gpt-a"].Value; got.ContextWindow != nil {
		t.Fatal("restore wrote the old typed value back into model settings")
	}
}

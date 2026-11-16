package aiservices

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"winone/internal/ai"
)

// Exercise the production page's save button and the real durable store after
// the same background write that formerly invalidated an idle page.
func TestIdlePageSavesDraftAndKeysAfterCatalogRefresh(t *testing.T) {
	fake, h, p, tt := setup(t)
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	provider := ai.Provider{ID: "one", Name: "Original", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"a"}}
	provider.RefreshModels([]ai.ModelOption{{ID: "a", Name: "A"}})
	configuration.Providers = []ai.Provider{provider}
	if _, err = store.SaveConfiguration(configuration, map[string]*string{"one": new("fixture-original")}); err != nil {
		t.Fatal(err)
	}
	var document document
	raw, _ := json.Marshal(store.Configuration())
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	p.accept(document)
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		if method == "capture.ai.configuration.save" {
			var request struct {
				Configuration ai.Configuration     `json:"configuration"`
				Base          *ai.Configuration    `json:"base"`
				KeyUpdates    ai.CredentialUpdates `json:"keyUpdates"`
			}
			if err := json.Unmarshal(params, &request); err != nil {
				return nil, err
			}
			return store.SaveConfigurationEdit(request.Configuration, request.Base, nil, request.KeyUpdates)
		}
		return fake.handle(method, params)
	})
	current := store.Configuration()
	current.Providers[0].RefreshModels([]ai.ModelOption{{ID: "a", Name: "Updated A"}, {ID: "b", Name: "New B"}})
	current.Providers[0].ModelRefreshAt = time.Unix(86400, 0).UTC()
	if _, err = store.SaveConfiguration(current, nil); err != nil {
		t.Fatal(err)
	}
	p.draft.Value.find("one").Name = "My provider"
	p.changeKey("one", "primary", new("fixture-edited"))
	settle(h, tt)
	h.Env.Rail.Save.Run()
	settle(h, tt)
	if p.err != "" || p.dirty() || p.saving || len(p.keys) != 0 {
		t.Fatal("idle page remained unusable", p.err)
	}
	if p.draft.Value.find("one").Name != "My provider" || len(p.draft.Value.find("one").available()) != 2 {
		t.Fatal("merged document was not returned to page")
	}
	if key, err := store.RevealCredential("one", "primary"); err != nil || key != "fixture-edited" {
		t.Fatal("key edit was lost", err)
	}
	// A second edit must use the returned revision, without a reload.
	p.draft.Value.find("one").Name = "Another edit"
	settle(h, tt)
	h.Env.Rail.Save.Run()
	settle(h, tt)
	if p.err != "" || p.dirty() || store.Configuration().Providers[0].Name != "Another edit" {
		t.Fatal("subsequent save failed", p.err)
	}
}

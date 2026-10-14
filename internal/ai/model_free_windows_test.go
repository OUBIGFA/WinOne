package ai

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFreeModelMetadataSurvivesSaveAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := store.Configuration()
	c.Engine = "llm"
	if err = json.Unmarshal([]byte(`[{"id":"workbuddy","name":"WorkBuddy","authMode":"account","agent":"workbuddy","agentBackend":"plugin","models":["model-a"],"availableModels":["model-a","model-b"],"modelNames":{"model-a":"Model A"},"modelFree":{"model-a":true,"model-b":false}}]`), &c.Providers); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(reopened.Configuration().Providers[0])
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err = json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}
	var free map[string]bool
	if err = json.Unmarshal(public["modelFree"], &free); err != nil || !free["model-a"] || free["model-b"] {
		t.Fatal("provider's explicit free/paid model metadata was lost", err)
	}
}

func TestFreeModelRulesMatchMagpie(t *testing.T) {
	provider := Provider{ModelFree: map[string]bool{"opaque": true}, ModelNames: map[string]string{"alias": "Vendor Free Model"}}
	for id, want := range map[string]bool{"opaque": true, "vendor/model:free": true, "FREE-model": true, "alias": true, "freedom-7b": false, "carefree": false, "unknown-price": false} {
		if got := provider.IsFreeModel(id); got != want {
			t.Errorf("%s free=%v, want %v", id, got, want)
		}
	}
}

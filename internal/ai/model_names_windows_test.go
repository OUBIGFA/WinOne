package ai

import (
	"path/filepath"
	"testing"
)

func TestModelDisplayNameDoesNotReplaceRoutingID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := store.Configuration()
	c.Engine = "llm"
	c.Providers = []Provider{{ID: "qoder", Name: "Qoder", AuthMode: "account", Agent: "qoder", AgentBackend: "plugin", Models: []string{"qmodel_38max"}, ModelNames: map[string]string{"qmodel_38max": "Qwen3.8-Max"}}}
	c.Tasks["default"] = Selection{Provider: "qoder", Model: "qmodel_38max"}
	saved, err := store.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved.Providers[0].ModelNames["qmodel_38max"] = "changed outside"
	p, _, err := store.ForTask("chat", nil)
	if err != nil || p.Model != "qmodel_38max" || p.ModelName != "Qwen3.8-Max" {
		t.Fatal("routing identity or name lost", p.Model, p.ModelName, err)
	}
	provider := store.Configuration().Providers[0]
	if provider.ModelIcon("qmodel_38max") != "qwen-color" {
		t.Fatal("alias model did not use its own icon")
	}
}

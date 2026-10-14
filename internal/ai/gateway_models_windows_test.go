//go:build windows

package ai

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGatewayModelChoicesSurviveRestartAndDoNotAliasCallers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "gateway.json")
	g := NewGateway(ctx, path, nil)
	defer g.Close()
	selected := Selection{Provider: "work", Model: "same"}
	other := Selection{Provider: "personal", Model: "same"}
	if !g.ModelFilter()(selected) {
		t.Fatal("legacy gateway lost its available models")
	}
	settings := g.Status().GatewaySettings
	settings.Models = []Selection{selected}
	saved, err := g.Save(settings, false)
	if err != nil {
		t.Fatal(err)
	}
	settings.Models[0] = other
	saved.Models[0] = other
	if !g.ModelFilter()(selected) || g.ModelFilter()(other) || g.Status().Models[0] != selected {
		t.Fatal("caller changed the saved model policy")
	}
	restored := NewGateway(ctx, path, nil)
	defer restored.Close()
	if !restored.ModelFilter()(selected) || restored.ModelFilter()(other) {
		t.Fatal("model choices were not restored")
	}
	before := restored.ModelFilter()
	empty := restored.Status().GatewaySettings
	empty.Models = []Selection{}
	if _, err = restored.Save(empty, false); err != nil {
		t.Fatal(err)
	}
	if restored.ModelFilter()(selected) || !before(selected) {
		t.Fatal("model snapshot changed in an admitted request")
	}
	none := NewGateway(ctx, path, nil)
	defer none.Close()
	if none.ModelFilter()(selected) || none.Status().Models == nil {
		t.Fatal("empty selection became the legacy all-model policy")
	}
}

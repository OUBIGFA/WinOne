package main

import (
	"encoding/json"
	"testing"

	"github.com/yetone/magpie/internal/gwtool"
	"github.com/yetone/magpie/internal/pluginrt"
	"github.com/yetone/magpie/internal/routes"
)

func TestBuiltinPluginsExposeCompleteRegistryAndApplySchema(t *testing.T) {
	t.Cleanup(func() { _ = applyRuntimeSettings(request{}) })
	if err := applyRuntimeSettings(request{Operation: "magpie.catalog", MagpieSettings: json.RawMessage(`{"builtinPreset":"all","builtinPlugins":{"gateway":{"config":{"callsPage":50}}}}`)}); err != nil {
		t.Fatal(err)
	}
	list, err := builtinPluginCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(pluginrt.Registered()) || len(list) < 14 {
		t.Fatalf("incomplete plugin registry: %d", len(list))
	}
	for _, p := range list {
		if p["id"] == "gateway" && p["config"].(map[string]any)["callsPage"] != int64(50) {
			t.Fatalf("schema value did not reach runtime: %#v", p["config"])
		}
	}
	for _, raw := range []string{`{"builtinPreset":"unknown"}`, `{"builtinPlugins":{"unknown":{"enabled":true}}}`, `{"builtinPlugins":{"gateway":{"config":{"callsPage":13}}}}`, `{"builtinPlugins":{"search":{"config":{"secret":"bad"}}}}`} {
		if err := applyRuntimeSettings(request{MagpieSettings: json.RawMessage(raw)}); err == nil {
			t.Fatalf("invalid settings accepted: %s", raw)
		}
	}
}

func TestBuiltinOffControlsActualGatewayServices(t *testing.T) {
	t.Cleanup(func() { _ = applyRuntimeSettings(request{}) })
	if err := applyRuntimeSettings(request{MagpieSettings: json.RawMessage(`{"builtinPlugins":{"search":{"enabled":false},"rotation":{"enabled":false}}}`)}); err != nil {
		t.Fatal(err)
	}
	stop := serveBuiltinPlugins()
	defer stop()
	if gwtool.Served(gwtool.WebSearch) {
		t.Fatal("disabled search still serves requests")
	}
	if _, ok := routes.Find("rotate"); ok {
		t.Fatal("disabled rotation still registered")
	}
	if err := applyRuntimeSettings(request{}); err != nil {
		t.Fatal(err)
	}
	if !gwtool.Served(gwtool.WebSearch) {
		t.Fatal("enabled search not served")
	}
	if _, ok := routes.Find("rotate"); !ok {
		t.Fatal("enabled rotation not restored")
	}
}

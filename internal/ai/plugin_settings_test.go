package ai

import (
	"encoding/json"
	"testing"
)

func TestPluginPreferencesValidateAndClone(t *testing.T) {
	var settings MagpieSettings
	if err := json.Unmarshal([]byte(`{"builtinPreset":"essentials","builtinPlugins":{"gateway":{"config":{"callsPage":50}},"rotation":{"enabled":false}},"bundledPlugins":{"calc":{"enabled":true,"options":{"precision":20}}},"pluginOptions":{"cursor":{"native-session":true}}}`), &settings); err != nil {
		t.Fatal(err)
	}
	if err := settings.validate(); err != nil {
		t.Fatal(err)
	}
	copy := settings.clone()
	copy.BuiltinPlugins["gateway"].Config["callsPage"] = 100
	copy.BundledPlugins["calc"].Options["precision"] = 10
	if settings.BuiltinPlugins["gateway"].Config["callsPage"] != float64(50) || settings.BundledPlugins["calc"].Options["precision"] != float64(20) {
		t.Fatal("plugin draft aliases saved values")
	}
	for _, raw := range []string{`{"builtinPreset":"missing"}`, `{"builtinPlugins":{"missing":{"enabled":true}}}`, `{"builtinPlugins":{"gateway":{"config":{"callsPage":7}}}}`, `{"builtinPlugins":{"usage":{"config":{"callsPage":20}}}}`, `{"bundledPlugins":{"../outside":{"enabled":true}}}`} {
		var invalid MagpieSettings
		_ = json.Unmarshal([]byte(raw), &invalid)
		if invalid.validate() == nil {
			t.Fatalf("invalid plugin preferences accepted: %s", raw)
		}
	}
}

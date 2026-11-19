package ai

import (
	"encoding/json"
	"testing"
)

func TestCodexClientSurvivesRuntimeSnapshot(t *testing.T) {
	var input MagpieSettings
	if err := json.Unmarshal([]byte(`{"codexClient":"desktop"}`), &input); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(input.clone())
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["codexClient"] != "desktop" {
		t.Fatalf("the selected Codex client was lost from runtime settings: %s", raw)
	}
	if saved["noStats"] != true || saved["noUsageStats"] != true {
		t.Fatal("request identity changed the retained recording boundary")
	}
}

//go:build windows

package ai

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestReasoningDisplayDefaultsOnAndPersistsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		data, _ := json.Marshal(s.Configuration())
		var view map[string]any
		_ = json.Unmarshal(data, &view)
		if view["showReasoning"] != want {
			t.Fatalf("showReasoning=%v, want %v", view["showReasoning"], want)
		}
	}
	check(true)
	data, _ := json.Marshal(s.Configuration())
	var values map[string]any
	_ = json.Unmarshal(data, &values)
	values["showReasoning"] = false
	data, _ = json.Marshal(values)
	var config Configuration
	_ = json.Unmarshal(data, &config)
	if _, err = s.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	check(false)
	// An older client omitting the new display field must not turn it back on.
	data, _ = json.Marshal(s.Configuration())
	_ = json.Unmarshal(data, &values)
	delete(values, "showReasoning")
	data, _ = json.Marshal(values)
	config = Configuration{}
	_ = json.Unmarshal(data, &config)
	if _, err = s.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	check(false)
}

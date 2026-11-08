package ai

import (
	"encoding/json"
	"testing"
)

func TestModelProtocolAndOwnHeadersSurviveSnapshot(t *testing.T) {
	for _, api := range []string{"chat", "responses", "anthropic", "gemini", "decide"} {
		var m ModelMetadata
		if err := json.Unmarshal([]byte(`{"api":"`+api+`","headers":{"X-Route":"model-a"}}`), &m); err != nil {
			t.Fatal(err)
		}
		if err := ValidateModelMetadata(m); err != nil {
			t.Errorf("%s: %v", api, err)
		}
		b, _ := json.Marshal(m.clone())
		var got map[string]json.RawMessage
		_ = json.Unmarshal(b, &got)
		if string(got["headers"]) != `{"X-Route":"model-a"}` {
			t.Errorf("%s snapshot lost model headers: %s", api, b)
		}
	}
}

package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifierTimeoutSurvivesSettings(t *testing.T) {
 var settings MagpieSettings
 if err := json.Unmarshal([]byte("{\"classifyTimeout\":45}"), &settings); err != nil { t.Fatal(err) }
 raw, _ := json.Marshal(settings.clone())
 if !strings.Contains(string(raw), "\"classifyTimeout\":45") { t.Fatalf("classifier timeout lost: %s", raw) }
 for _, value := range []string{"-1", "61"} {
  if err := json.Unmarshal([]byte("{\"classifyTimeout\":"+value+"}"), &settings); err != nil { t.Fatal(err) }
  if settings.validate() == nil { t.Fatalf("invalid classifier timeout accepted: %s", value) }
 }
}

func TestMagpieRequestSettingsSurvivePrivateSnapshot(t *testing.T) {
	var settings MagpieSettings
	if err := json.Unmarshal([]byte("{\"claudeLeanPrompt\":true,\"fetchPrivate\":[\"nas.lan:8080\",\"192.168.1.0/24\"]}"), &settings); err != nil {
		t.Fatal(err)
	}
	if err := settings.validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(settings.clone())
	if !strings.Contains(string(raw), "\"claudeLeanPrompt\":true") || !strings.Contains(string(raw), "nas.lan:8080") {
		t.Fatalf("new settings lost: %s", raw)
	}
}

func TestMagpiePrivateFetchEntriesRejectAmbiguousAddresses(t *testing.T) {
	for _, entry := range []string{"", "https://nas.lan/private", "nas.lan\nother.lan", strings.Repeat("a", 513)} {
		raw, _ := json.Marshal(map[string]any{"fetchPrivate": []string{entry}})
		var settings MagpieSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatal(err)
		}
		if settings.validate() == nil {
			t.Fatalf("invalid private host accepted: %q", entry)
		}
	}
}

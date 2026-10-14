//go:build windows

package main

import (
	"testing"
	"winone/internal/ai"
)

func TestChatModelCarriesPercentageAndReportedContextBounds(t *testing.T) {
	config := ai.Configuration{Providers: []ai.Provider{{ID: "provider", Name: "Provider", Models: []string{"model"}, ModelMetadata: map[string]ai.ModelMetadata{"model": {ContextWindow: 256000, CompactionPercent: 72.5, CompactionThreshold: 100000, ListedContextWindow: 128000, MaxContextWindow: 1000000}}}}}
	model, err := chatModel(config, ai.Selection{Provider: "provider", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if model["compactionPercent"] != 72.5 || model["compactionThreshold"] != 100000 || model["contextWindow"] != 256000 || model["listedContextWindow"] != 128000 || model["maxContextWindow"] != 1000000 {
		t.Fatalf("Pi model bridge dropped settings: %+v", model)
	}
}

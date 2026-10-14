package main

import (
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"os"
	"strings"
	"testing"
)

func TestAccountCatalogIncludesCommunityProvidersWithoutDuplicateServices(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "catalog-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previous })
	if err := accountOperation(context.Background(), request{Operation: "accounts.catalog"}, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Agents []agentOption `json:"agents"`
	}
	if err := json.NewDecoder(file).Decode(&result); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, agent := range result.Agents {
		if seen[agent.ID] || agent.ID == "" || agent.Name == "" {
			t.Fatalf("invalid or duplicate service: %+v", agent)
		}
		seen[agent.ID] = true
	}
	for _, id := range []string{"codex", "claude", "workbuddy-ai", "cline", "opencode-zen-free", "minimax-code", "minimax-code-global", "trae-cn", "trae-global"} {
		if !seen[id] {
			t.Errorf("account catalog omits %s", id)
		}
	}
	if seen["model-map"] || seen["word-guard"] {
		t.Fatal("middleware was presented as an account")
	}
	for _, id := range provider.WinOneAccountIDs() {
		if !seen[id] {
			t.Errorf("upstream built-in account is missing: %s", id)
		}
	}
}

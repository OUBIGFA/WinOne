package main

import (
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"os"
	"slices"
	"strings"
	"testing"
)

// Recognize the retained migration operation, and refuse an empty private
// profile before it could load or install anything outside that profile.
func TestAntigravityMigrationRequiresAnExistingPrivateAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := applyRuntimeSettings(request{}); err != nil {
		t.Fatal(err)
	}
	err := accountOperation(t.Context(), request{Operation: "accounts.move-agy", Agent: "antigravity"}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "no Antigravity account is left to move") {
		t.Fatalf("private migration unavailable: %v", err)
	}
}

// A bundled withdrawn adapter remains usable for saved accounts, but must
// never become an option for a newly configured provider.
func TestWithdrawnAccountAdapterIsNotOfferedToNewProviders(t *testing.T) {
	if slices.ContainsFunc(accountCatalog(), func(a agentOption) bool { return a.ID == "antigravity" }) {
		t.Fatal("withdrawn Antigravity adapter is offered to new providers")
	}
	if _, ok := pluginForAgent("antigravity"); !ok {
		t.Fatal("saved Antigravity providers lost their bundled adapter")
	}
	if !slices.ContainsFunc(accountCatalog(), func(a agentOption) bool { return a.ID == "agy-cli" }) {
		t.Fatal("replacement account adapter is not offered")
	}
}

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
		if p, ok := pluginForAgent(id); ok && p.Legacy {
			continue
		}
		if !seen[id] {
			t.Errorf("upstream built-in account is missing: %s", id)
		}
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// These failures are decided by the actual upstream warm-up entry points;
// neither a missing operation nor an invented generic account action counts.
func TestAccountWarmupUsesUpstreamSupportedAccountRules(t *testing.T) {
	old := provider.WinOneProviders
	t.Cleanup(func() { provider.WinOneProviders = old })
	for _, operation := range []string{"accounts.warm-now", "accounts.warm-all"} {
		err := accountOperation(t.Context(), request{Operation: operation, Agent: "copilot", User: "person@example.invalid"}, strings.NewReader(""))
		if err == nil || !strings.Contains(err.Error(), "has no warm-up") {
			t.Fatalf("%s did not reach the upstream warm-up rule: %v", operation, err)
		}
	}
}

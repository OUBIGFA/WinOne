package aiservices

import (
	"fmt"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
)

// TestShots saves every tab of the page in both themes and two widths
// when WINONE_SHOTS names a folder, for looking at.
func TestShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	_, h, p, _ := setup(t)
	t.Cleanup(func() {
		if t.Failed() {
			calls := map[string]int{}
			for _, call := range h.Calls() {
				calls[call.Method]++
			}
			t.Logf("last tab=%d requests=%v", p.tab, calls)
		}
	})
	for i, label := range []string{i18n.T("ai.customProviders"), i18n.T("ai.addProvidersTab"), homeT("preferences"), upstreamT("Gateway"), upstreamT("Routing"), upstreamT("Usage"), upstreamT("Plugins"), i18n.T("ai.advanced")} {
		h.Shots(t, fmt.Sprintf("aiservices-%d", i), p.View, 1600, func(tt *ui.Tester) {
			if err := tt.Click(label); err != nil {
				t.Fatalf("tab %q: %v", label, err)
			}
		})
	}
}

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
	for i, key := range []string{"ai.customProviders", "ai.addProvidersTab", "ai.defaultService", "gateway.title", "routing.title", "usage.title", "aiPlugins.title", "ai.advanced"} {
		label := i18n.T(key)
		h.Shots(t, fmt.Sprintf("aiservices-%d", i), p.View, 1600, func(tt *ui.Tester) {
			if err := tt.Click(label); err != nil {
				t.Logf("tab %q: %v", label, err)
			}
		})
	}
}

package selection

import (
	"fmt"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
)

// TestShots saves every tab of both assistants in both themes and two
// widths when WINONE_SHOTS names a folder, for looking at.
func TestShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	for _, module := range []string{"selection", "quick-assistant"} {
		document := selectionDocument()
		if module == "quick-assistant" {
			document = quickDocument()
		}
		_, h, p, _ := setup(t, module, "running", document)
		for i, key := range []string{"selection.toolbar", "selection.general", "shortcuts.title"} {
			label := i18n.T(key)
			h.Shots(t, fmt.Sprintf("%s-%d", module, i), p.View, 1600, func(tt *ui.Tester) {
				if err := tt.Click(label); err != nil {
					t.Logf("tab %q: %v", label, err)
				}
			})
		}
	}
}

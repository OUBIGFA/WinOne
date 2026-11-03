package interconnect

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
	i18n.SetLocale(i18n.Chinese)
	var calls []call
	h := interconnectHost(t, general{Notify: true, DownloadDir: `C:\Downloads`}, &calls)
	p := newPage(h.Env)
	for i, label := range []string{tr("fileTransfer"), tr("store"), tr("tunnel"), tr("general")} {
		h.Shots(t, fmt.Sprintf("interconnect-%d", i), p.View, 1600, func(tt *ui.Tester) {
			if err := tt.Click(label); err != nil {
				t.Logf("tab %q: %v", label, err)
			}
		})
	}
}

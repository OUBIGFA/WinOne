package shell

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
	"winone/internal/timedpower"
)

// The running task must remain visible outside Convenience, including the
// default collapsed sidebar. The Host is reached over the real pipe client.
func TestTimedPowerVisibleAcrossPagesAndSidebarWidths(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, width := range []float32{64, 240} {
			for _, mode := range []string{"countdown", "time", "weekly"} {
				i18n.SetLocale(locale)
				p := timedpower.Defaults()
				p.Mode = mode
				p.DryRun = true
				h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
					if method == "timed-power.snapshot" {
						return timedpower.Snapshot{Active: true, State: "running", Task: &p, Deadline: time.Now().Add(time.Hour)}, nil
					}
					return map[string]any{}, nil
				})
				s := New(h.Env, &feedbackWindow{}, Options{Directory: h.Env.Directory})
				t.Cleanup(s.Discarded)
				s.sidebarWidth = width
				s.apply(internalhost.Snapshot{Modules: []internalhost.Module{{ID: "timed-power", State: "running"}}})
				i18n.SetLocale(locale)
				for _, page := range []string{"overview", "settings"} {
					s.page = page
					s.instance = core.PageFunc(func(c *ui.Context) { ui.Text(c, "Page content") })
					tt := ui.NewTester(s.View, 1000, 800)
					h.Settle(tt)
					if _, ok := tt.Access(i18n.T("timedPower.remaining")); !ok {
						t.Fatalf("no global remaining time: locale=%s width=%v mode=%s page=%s", locale, width, mode, page)
					}
				}
			}
		}
	}
}

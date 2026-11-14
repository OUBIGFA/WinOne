package convenience

import (
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/internal/timedpower"
)

// A preset must update the rendered fields during its own input cycle. An
// extra Frame or Host settle here would hide a stale field until another event.
func TestTimedPowerPresetsUpdateVisibleFieldsImmediately(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, width := range []int{560, 1000} {
			t.Run(locale+"/"+strconv.Itoa(width), func(t *testing.T) {
				i18n.SetLocale(locale)
				draft := toDraft(timedpower.Defaults())
				draft.delay = clock{0, 0, 17}
				power := &powerTab{draft: &draft}
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), false))
					power.schedule(c)
				}, width, 500)
				for _, minutes := range []int{15, 30, 60, 90, 120, 150, 15} {
					label := i18n.T("timedPower.preset" + strconv.Itoa(minutes))
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					for i, part := range []string{"hours", "minutes", "seconds"} {
						want := []int{minutes / 60, minutes % 60, 0}[i]
						field, ok := tt.Access(i18n.T("timedPower." + part))
						if !ok || field.Value != strconv.Itoa(want) {
							t.Fatalf("after %s: visible %s = %q, want %d (draft %v)", label, part, field.Value, want, draft.delay)
						}
					}
				}
				// Continue from the last clicked preset with keyboard navigation.
				tt.Key(0, ui.KeyTab)
				if !tt.Focused(i18n.T("timedPower.preset30")) {
					t.Fatal("Tab did not reach the next preset")
				}
				tt.Key(0, ui.KeyEnter)
				if field, _ := tt.Access(i18n.T("timedPower.minutes")); field.Value != "30" || draft.delay != (clock{0, 30, 0}) {
					t.Fatalf("keyboard preset left stale fields: %+v, draft=%v", field, draft.delay)
				}
			})
		}
	}
}

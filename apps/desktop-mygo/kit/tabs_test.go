package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTaskTabsRemainReachableWhenWrappedAndTextIsEnlarged(t *testing.T) {
	labels := []Tab{{Label: "Custom providers"}, {Label: "Account providers"}, {Label: "Default AI service"}, {Label: "Configuration transfer"}, {Label: "Advanced settings"}}
	for _, scale := range []float32{1, 2} {
		selected := 0
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(Theme(ui.Preferences{TextScale: scale}, false))
			Tabs(c, &selected, labels, nil)
		}, 360, 600)
		for i, tab := range labels {
			bounds, ok := tt.Find(tab.Label)
			if !ok || bounds.X < 0 || bounds.X+bounds.W > 360 || bounds.Y+bounds.H > 600 {
				t.Fatalf("scale %v: inaccessible task %q at %+v", scale, tab.Label, bounds)
			}
			if err := tt.Click(tab.Label); err != nil || selected != i {
				t.Fatalf("scale %v: task %q cannot be selected: %v", scale, tab.Label, err)
			}
		}
	}
}

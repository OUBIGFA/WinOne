package resultui

import (
	"testing"

	"winone/internal/resultwindow"
)

// MyGo owns native styles now; test the production options rather than a
// removed Win32 patch. Real-window style checks belong to the window probe.
func TestSelectionWindowNeverQualifiesForTaskbar(t *testing.T) {
	for _, toolbar := range []bool{true, false} {
		c := &Controller{Initial: resultwindow.Initial{Source: "selection"}, Panel: NewPanel("ocr", "zh-CN", "light"), Toolbar: toolbar}
		o := c.WindowOptions()
		if !o.Panel || !o.AlwaysOnTop || o.DisableResize != toolbar {
			t.Fatalf("selection toolbar=%v: %+v", toolbar, o)
		}
		if toolbar {
			w, h := c.ToolbarDimensions()
			if o.Width != w || o.Height != h || o.MinWidth != 0 || o.MinHeight != 0 {
				t.Fatalf("toolbar geometry: %+v", o)
			}
		} else if o.Width != DefaultResultWidth || o.Height != DefaultResultHeight || o.MinWidth != 320 || o.MinHeight != 300 {
			t.Fatalf("result geometry: %+v", o)
		}
	}
}

func TestResultWindowOptionsPreserveVisibilityAndPinning(t *testing.T) {
	for _, source := range []string{"capture", "quick-assistant"} {
		for _, pinned := range []bool{false, true} {
			c := &Controller{Initial: resultwindow.Initial{Source: source}, Panel: NewPanel("ocr", "en-US", "dark")}
			c.Panel.Pinned = pinned
			o := c.WindowOptions()
			if o.Panel || !o.Hidden || !o.Frameless || !o.UseContentSize || !o.DisableMaximize || o.DisableResize || o.CornerRadius != 8 {
				t.Fatalf("result options: %+v", o)
			}
			if o.AlwaysOnTop != (source != "quick-assistant" || pinned) {
				t.Fatalf("pin policy source=%s pinned=%v", source, pinned)
			}
		}
	}
}

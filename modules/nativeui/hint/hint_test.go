package hint

import (
	"github.com/egoist/mygo/ui"
	"math"
	"testing"
	"time"
)

func TestHintContrastAndActualHoverText(t *testing.T) {
	luminance := func(c ui.Color) float64 {
		channel := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= .04045 {
				return x / 12.92
			}
			return math.Pow((x+.055)/1.055, 2.4)
		}
		return .2126*channel(c.R) + .7152*channel(c.G) + .0722*channel(c.B)
	}
	for _, dark := range []bool{false, true} {
		bg, fg, _ := Colors(dark)
		a, b := luminance(bg), luminance(fg)
		if a > b {
			a, b = b, a
		}
		if (b+.05)/(a+.05) < 7 {
			t.Fatalf("insufficient tooltip contrast: dark=%v", dark)
		}
		tt := ui.NewTester(func(c *ui.Context) {
			theme := *c.Theme()
			theme.Dark = dark
			c.SetTheme(&theme)
			ui.Column(c).Fill().Padding(70).Children(func() {
				button := ui.ButtonBase(c).Size(40, 32).Label("action").TextColor(ui.Hex("#292927"))
				Attach(c, button, "输入说明 Tooltip")
			})
		}, 400, 220)
		tt.Advance(0)
		rect, ok := tt.Find("action")
		if !ok {
			t.Fatal("button missing")
		}
		tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
		tt.Advance(time.Second)
		if !tt.HasText("输入说明 Tooltip") {
			t.Fatal("hover text missing")
		}
		image := tt.Image()
		light, darkPixels := 0, 0
		for y := 0; y < 65; y++ {
			for x := 0; x < 400; x++ {
				p := image.RGBAAt(x, y)
				if p.R > 230 && p.G > 230 && p.B > 230 {
					light++
				}
				if p.R < 60 && p.G < 60 && p.B < 60 {
					darkPixels++
				}
			}
		}
		if darkPixels < 50 || light < 10 {
			t.Fatalf("actual popup did not contain contrast pair: dark=%d light=%d", darkPixels, light)
		}
	}
}

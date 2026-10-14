package resultui

import (
	"bytes"
	"fmt"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"image/png"
	"strings"
)

// Brand SVGs can contain several coincident paths (for gradient overlays).
// Rasterizing them directly at 16px hardens edge coverage. Build-time resvg
// samples the original at 128px; mygo's mip levels/filtering then preserve
// smooth coverage at the actual DPI without a renderer process at runtime.
func providerBitmap(name string, theme *ui.Theme) *ui.Bitmap {
	if name == "" || strings.ContainsAny(name, "/\\.") {
		return nil
	}
	monochrome := theme.Dark || !strings.HasSuffix(name, "-color")
	key := "provider-bitmap/" + name
	if monochrome {
		key += fmt.Sprintf("/%v", theme.TextMuted)
	}
	if value, ok := iconCache.Load(key); ok {
		return value.(*ui.Bitmap)
	}
	source, err := assets.Open("assets/providers/" + name + ".svg")
	if err != nil {
		return nil
	}
	_ = source.Close()
	data, err := assets.ReadFile("assets/provider-bitmaps/" + name + ".png")
	if err != nil {
		panic(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	if monochrome {
		tinted := image.NewNRGBA(decoded.Bounds())
		ink := theme.TextMuted
		for y := tinted.Rect.Min.Y; y < tinted.Rect.Max.Y; y++ {
			for x := tinted.Rect.Min.X; x < tinted.Rect.Max.X; x++ {
				_, _, _, alpha := decoded.At(x, y).RGBA()
				tinted.SetNRGBA(x, y, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: uint8((alpha*uint32(ink.A) + 32767) / 65535)})
			}
		}
		decoded = tinted
	}
	bitmap := ui.NewBitmap(decoded)
	actual, _ := iconCache.LoadOrStore(key, bitmap)
	return actual.(*ui.Bitmap)
}

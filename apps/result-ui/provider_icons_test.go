package resultui

import (
	"bytes"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestBundledProviderArtworkRenders(t *testing.T) {
	entries, err := assets.ReadDir("assets/providers")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".svg") {
			names = append(names, strings.TrimSuffix(entry.Name(), ".svg"))
		}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Wrap().Gap(12).Padding(12).Background(ui.Hex("#ffffff")).Children(func() {
			for _, name := range names {
				ui.Column(c).Width(120).Height(86).Center().Children(func() { ui.Image(c, providerSVG(name)).Size(36, 36); ui.Text(c, name).FontSize(12) })
			}
		})
	}, 816, 330)
	if path := os.Getenv("WINONE_PROVIDER_ICON_PREVIEW"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err = png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKimiLightArtworkKeepsVisibleBodyAndBlueAccent(t *testing.T) {
	raw, err := assets.ReadFile("assets/provider-bitmaps/kimi-color.png")
	if err != nil {
		t.Fatal(err)
	}
	icon, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	dark, blue := 0, 0
	for y := 0; y < icon.Bounds().Dy(); y++ {
		for x := 0; x < icon.Bounds().Dx(); x++ {
			r, g, b, a := icon.At(x, y).RGBA()
			if a < 0xe000 {
				continue
			}
			if r < 0x6000 && g < 0x6000 && b < 0x6000 {
				dark++
			}
			if b > r+0x4000 && b > g+0x1000 {
				blue++
			}
		}
	}
	if dark < 1200 || blue < 100 {
		t.Fatalf("Kimi is not legible on white: body=%d accent=%d", dark, blue)
	}
}

func TestProviderArtworkUsesColorOnlyInLightTheme(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("ocr", "zh-CN", theme)
		tt := ui.NewTester(func(c *ui.Context) {
			palette := p.palette(c)
			c.SetTheme(palette)
			ui.Column(c).Fill().Background(palette.Background).Center().Children(func() { providerImage(c, palette, "deepseek-color", 40) })
		}, 80, 80)
		colored := 0
		pixels := tt.Image()
		for y := 0; y < 80; y++ {
			for x := 0; x < 80; x++ {
				r, g, b, _ := pixels.At(x, y).RGBA()
				if r != g || g != b {
					colored++
				}
			}
		}
		if theme == "light" && colored == 0 || theme == "dark" && colored != 0 {
			t.Fatalf("provider theme %s has %d colored pixels", theme, colored)
		}
	}
}

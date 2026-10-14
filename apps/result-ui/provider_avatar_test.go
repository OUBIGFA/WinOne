package resultui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/egoist/mygo/ui"
	"image"
	"image/color"
	"image/png"
	"testing"
	"winone/internal/ai"
)

func avatarPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 17, 13))
	picture.SetNRGBA(4, 5, color.NRGBA{R: 180, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestCurrentCustomAvatarWinsOverHistoricalBrandAndOldPicture(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "dark")
	avatar := "file:0123456789abcdef.png"
	p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "custom", Name: "Custom", Icon: "openai", Avatar: &avatar}}}
	message := Message{Provider: "custom", ProviderName: "Historical name", ProviderIcon: "deepseek-color", Model: "historical-model"}
	if got := p.messageProviderIcon(&message); got != avatar {
		t.Fatal("brand icon replaced custom picture", got)
	}
	updated := "file:1123456789abcdef.png"
	p.Configuration.Providers[0].Avatar = &updated
	if got := p.messageProviderIcon(&message); got != updated {
		t.Fatal("historical cached icon replaced latest picture", got)
	}
	if message.ProviderName != "Historical name" || message.Model != "historical-model" {
		t.Fatal("artwork refresh changed call identity")
	}
}

func TestCustomSVGUsesOriginalCurrentColorInBothThemes(t *testing.T) {
	avatar, err := decodeProviderAvatar([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("ocr", "zh-CN", theme)
		p.avatars = map[string]*providerAvatar{"file:0123456789abcdef.svg": avatar}
		tt := ui.NewTester(func(c *ui.Context) {
			palette := p.palette(c)
			c.SetTheme(palette)
			ui.Column(c).Fill().Background(palette.Background).Center().Children(func() { p.providerImage(c, palette, "file:0123456789abcdef.svg", 32) })
		}, 48, 48)
		r, g, b, _ := tt.Image().At(24, 24).RGBA()
		if r != 0 || g != 0 || b != 0 {
			t.Fatalf("custom SVG changed color in %s: %d %d %d", theme, r, g, b)
		}
	}
}

func TestRefreshingServicesRetriesFailedCustomAvatar(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	calls := 0
	name := "file:0123456789abcdef.png"
	p.Actions.loadAvatar = func(_ string, done func(*providerAvatar, error)) {
		calls++
		if calls == 1 {
			done(nil, errors.New("Temporary failure"))
			return
		}
		value, err := decodeProviderAvatar(avatarPNG(t))
		done(value, err)
	}
	render := func() { ui.NewTester(func(c *ui.Context) { p.providerImage(c, p.palette(c), name, 32) }, 48, 48) }
	render()
	p.retryProviderAvatars()
	render()
	if calls != 2 || p.avatars[name].bitmap == nil {
		t.Fatal("refresh retained a failed avatar cache entry", calls)
	}
}
func avatarICO(png []byte) []byte {
	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], 1)
	header[6] = 17
	header[7] = 13
	binary.LittleEndian.PutUint32(header[14:], uint32(len(png)))
	binary.LittleEndian.PutUint32(header[18:], 22)
	return append(header, png...)
}

func TestProviderAvatarDecodesPNGICOAndSVG(t *testing.T) {
	picture := avatarPNG(t)
	for _, data := range [][]byte{picture, avatarICO(picture)} {
		avatar, err := decodeProviderAvatar(data)
		if err != nil {
			t.Fatal(err)
		}
		if avatar.bitmap == nil {
			t.Fatal("picture not decoded")
		}
		w, h := avatar.bitmap.Size()
		if w != 17 || h != 13 {
			t.Fatalf("picture proportions changed: %dx%d", w, h)
		}
	}
	avatar, err := decodeProviderAvatar([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12"><circle cx="6" cy="6" r="5" fill="red"/></svg>`))
	if err != nil || avatar.svg == nil {
		t.Fatal("SVG unavailable", err)
	}
}

func TestProviderAvatarRejectsDamagedICOWithoutAllocatingItsClaimedSize(t *testing.T) {
	bad := avatarICO(avatarPNG(t))
	binary.LittleEndian.PutUint32(bad[14:], 0xfffffff0)
	for _, data := range [][]byte{bad, bad[:12], []byte("not an image")} {
		if _, err := decodeProviderAvatar(data); err == nil {
			t.Fatal("damaged picture was accepted")
		}
	}
}

func TestProviderAvatarKeepsOpaquePixelsIn256PixelBitmapICO(t *testing.T) {
	// ICO's zero width/height byte means 256, not an empty mask. This DIB is a
	// solid opaque picture, so a decoder losing its alpha would render white.
	data := make([]byte, 22+40+256*256*4)
	binary.LittleEndian.PutUint16(data[2:], 1)
	binary.LittleEndian.PutUint16(data[4:], 1)
	binary.LittleEndian.PutUint16(data[10:], 1)
	binary.LittleEndian.PutUint16(data[12:], 32)
	binary.LittleEndian.PutUint32(data[14:], uint32(len(data)-22))
	binary.LittleEndian.PutUint32(data[18:], 22)
	dib := data[22:]
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], 256)
	binary.LittleEndian.PutUint32(dib[8:], 512)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	for at := 40; at < len(dib); at += 4 {
		dib[at], dib[at+1], dib[at+2], dib[at+3] = 40, 70, 190, 255
	}
	avatar, err := decodeProviderAvatar(data)
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(ui.Hex("#ffffff")).Children(func() { ui.Image(c, avatar.bitmap).Size(32, 32) })
	}, 32, 32)
	r, g, b, _ := tester.Image().At(16, 16).RGBA()
	if r>>8 != 190 || g>>8 != 70 || b>>8 != 40 {
		t.Fatalf("ICO lost pixels/alpha: %d %d %d", r>>8, g>>8, b>>8)
	}
}

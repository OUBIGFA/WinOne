package widgets

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
	resultassets "winone/resultui/assets"
)

// ProviderIcon shows the mark of an AI provider or model family, size
// pixels square, as React's AIProviderIcon did:
//   - "file:<hash>.<ext>" is the user's own picture, read once through the
//     Host and kept; a generic service mark shows while it loads or when it
//     cannot be read.
//   - "model" is a model of no known family: Sparkle.
//   - a built-in name shows its 128 px sample from the result window's
//     inventory: in its colors when the name ends in "-color" and the theme
//     is light, else in the text color. Marks that exist only as pictures
//     keep their colors, gray in the dark theme.
//   - anything else is a generic service: Layers.
//
// The marks are decorations; the name beside them is the label.
func ProviderIcon(c *ui.Context, env *core.Env, icon string, size float32) ui.Element {
	t := c.Theme()
	generic := func(svg *ui.SVG) ui.Element {
		return ui.Icon(c, svg).Size(size, size).Shrink(0).TextColor(t.TextMuted).Role(ui.RoleNone)
	}
	if ai.IsStoredAvatar(icon) {
		entry := avatars.get(env, icon)
		switch {
		case entry.svg != nil:
			return ui.Image(c, entry.svg).Size(size, size).Fit(ui.Contain).Shrink(0).Role(ui.RoleNone)
		case entry.bitmap != nil:
			return ui.Image(c, entry.bitmap).Size(size, size).Fit(ui.Contain).Shrink(0).Role(ui.RoleNone)
		case entry.failed:
			return generic(icons.Layers).Tooltip(i18n.T("ai.avatarReadError"))
		}
		return generic(icons.Layers)
	}
	if icon == "model" {
		return generic(icons.Sparkle)
	}
	if bitmap := builtInProviderBitmap(icon, t); bitmap != nil {
		return ui.Image(c, bitmap).Size(size, size).Fit(ui.Contain).Shrink(0).Role(ui.RoleNone)
	}
	return generic(icons.Layers)
}

// ModelIcon shows a model's family mark, as React's AIModelIcon did: by
// its ID, or by its display name when the ID matches no family.
func ModelIcon(c *ui.Context, env *core.Env, model, name string, size float32) ui.Element {
	icon := ai.ModelIcon(model)
	if icon == "model" && name != "" {
		icon = ai.ModelIcon(name)
	}
	return ProviderIcon(c, env, icon, size)
}

// BuiltInProviderIcons lists the built-in marks a provider can use, by
// name with its "-color" variant beside it.
func BuiltInProviderIcons() []string {
	var names []string
	entries, _ := fs.ReadDir(resultassets.Providers, "providers")
	for _, entry := range entries {
		name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".svg"), ".png")
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	base := func(name string) string { return strings.TrimSuffix(name, "-color") }
	slices.SortStableFunc(names, func(a, b string) int { return strings.Compare(base(a), base(b)) })
	return names
}

// RefreshProviderAvatars reads again the pictures that could not be read,
// as after the AI services are refreshed; pictures read once never change,
// as their names are their content's hash.
func RefreshProviderAvatars() { avatars.retry() }

var providerBitmaps sync.Map

// builtInProviderBitmap returns the sample of a built-in mark for the
// theme, nil for an unknown name.
func builtInProviderBitmap(name string, t *ui.Theme) *ui.Bitmap {
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return nil
	}
	file := "provider-bitmaps/" + name + ".png"
	picture := false
	if _, err := fs.Stat(resultassets.Providers, "providers/"+name+".svg"); err != nil {
		// Marks without artwork exist only as pictures, shown as they are.
		file, picture = "providers/"+name+".png", true
		if _, err := fs.Stat(resultassets.Providers, file); err != nil {
			return nil
		}
	}
	monochrome := !picture && (t.Dark || !strings.HasSuffix(name, "-color"))
	gray := picture && t.Dark
	key := file
	switch {
	case monochrome:
		key += "#" + colorKey(t.Text)
	case gray:
		key += "#gray"
	}
	if cached, ok := providerBitmaps.Load(key); ok {
		return cached.(*ui.Bitmap)
	}
	data, err := fs.ReadFile(resultassets.Providers, file)
	if err != nil {
		return nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	switch {
	case monochrome:
		decoded = tint(decoded, t.Text)
	case gray:
		decoded = grayscale(decoded)
	}
	bitmap, _ := providerBitmaps.LoadOrStore(key, ui.NewBitmap(decoded))
	return bitmap.(*ui.Bitmap)
}

func colorKey(c ui.Color) string { return fmt.Sprintf("%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }

// tint keeps a sample's coverage in one color, as a CSS mask in the text
// color did.
func tint(source image.Image, ink ui.Color) image.Image {
	bounds := source.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: uint8((alpha*uint32(ink.A) + 32767) / 65535)})
		}
	}
	return out
}

// grayscale is CSS grayscale(1): the luminance of each pixel, alpha kept.
func grayscale(source image.Image) image.Image {
	bounds := source.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			l := uint8((2126*uint32(pixel.R) + 7152*uint32(pixel.G) + 722*uint32(pixel.B) + 5000) / 10000)
			out.SetNRGBA(x, y, color.NRGBA{R: l, G: l, B: l, A: pixel.A})
		}
	}
	return out
}

// avatarStore keeps the users' pictures by name, shared by every list
// and menu, as React's provider-avatar-cache did. It is used on the UI
// thread only.
type avatarStore struct {
	entries map[string]*avatarEntry
	order   []string
}

type avatarEntry struct {
	bitmap  *ui.Bitmap
	svg     *ui.SVG
	failed  bool
	loading bool
}

var avatars avatarStore

const (
	avatarChunk   = 256 << 10
	avatarEntries = 64
)

func (s *avatarStore) get(env *core.Env, icon string) *avatarEntry {
	if entry := s.entries[icon]; entry != nil {
		return entry
	}
	if s.entries == nil {
		s.entries = map[string]*avatarEntry{}
	}
	if len(s.order) >= avatarEntries {
		delete(s.entries, s.order[0])
		s.order = s.order[1:]
	}
	entry := &avatarEntry{loading: true}
	s.entries[icon] = entry
	s.order = append(s.order, icon)
	core.Go(env, func() (ui.ImageSource, error) {
		data, err := readAvatar(env, icon)
		if err != nil {
			return nil, err
		}
		return decodeAvatar(data)
	}, func(picture ui.ImageSource, err error) {
		entry.loading = false
		switch value := picture.(type) {
		case *ui.Bitmap:
			entry.bitmap = value
		case *ui.SVG:
			entry.svg = value
		default:
			entry.failed = true
		}
	})
	return entry
}

func (s *avatarStore) retry() {
	for name, entry := range s.entries {
		if entry.failed {
			delete(s.entries, name)
			s.order = slices.DeleteFunc(s.order, func(n string) bool { return n == name })
		}
	}
}

var avatarTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/svg+xml"}

// readAvatar reads a stored picture in chunks, checking that every chunk
// belongs to the same picture.
func readAvatar(env *core.Env, icon string) ([]byte, error) {
	var data []byte
	size, mimeType := 0, ""
	for {
		var chunk struct {
			Data     string `json:"data"`
			Size     int    `json:"size"`
			MimeType string `json:"mimeType"`
		}
		if err := env.Host.CallInto("capture.ai.icons.read", map[string]any{"icon": icon, "offset": len(data)}, &chunk); err != nil {
			return nil, err
		}
		if chunk.Size <= 0 || chunk.Size > ai.MaxAvatarBytes || !slices.Contains(avatarTypes, chunk.MimeType) ||
			size != 0 && size != chunk.Size || mimeType != "" && mimeType != chunk.MimeType {
			return nil, errors.New("头像数据无效")
		}
		size, mimeType = chunk.Size, chunk.MimeType
		part, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil || len(part) == 0 || len(part) > avatarChunk || len(data)+len(part) > size {
			return nil, errors.New("头像数据不完整")
		}
		data = append(data, part...)
		if len(data) >= size {
			return data, nil
		}
	}
}

// decodeAvatar decodes a stored picture: an SVG, an icon file's largest
// image, or a bitmap of bounded size.
func decodeAvatar(data []byte) (ui.ImageSource, error) {
	invalid := errors.New("头像图片无法解码，请重新选择图片")
	if len(data) == 0 || len(data) > ai.MaxAvatarBytes {
		return nil, invalid
	}
	if bytes.Contains(bytes.ToLower(data[:min(1024, len(data))]), []byte("<svg")) {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return nil, invalid
		}
		return svg, nil
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0, 0, 1, 0}) {
		picture, err := icoImage(data)
		if err != nil {
			return nil, invalid
		}
		return ui.NewBitmap(picture), nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > 16_000_000 {
		return nil, invalid
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, invalid
	}
	return bitmap, nil
}

// icoImage decodes the largest image of an icon file: a PNG, or a device
// independent bitmap with its transparency mask. The standard library
// reads neither; a browser did for the React window.
func icoImage(data []byte) (image.Image, error) {
	invalid := errors.New("invalid icon")
	if len(data) < 6 {
		return nil, invalid
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 64 || len(data) < 6+16*count {
		return nil, invalid
	}
	var payload []byte
	best := -1
	for i := range count {
		entry := data[6+16*i : 6+16*(i+1)]
		size := uint64(binary.LittleEndian.Uint32(entry[8:12]))
		offset := uint64(binary.LittleEndian.Uint32(entry[12:16]))
		if size < 8 || offset < uint64(6+16*count) || offset+size > uint64(len(data)) {
			return nil, invalid
		}
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if score := width * height; score > best {
			best, payload = score, data[offset:offset+size]
		}
	}
	if bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
		picture, _, err := image.Decode(bytes.NewReader(payload))
		if err != nil || picture.Bounds().Dx() > 4096 || picture.Bounds().Dy() > 4096 {
			return nil, invalid
		}
		return picture, nil
	}
	return dibImage(payload)
}

// dibImage decodes an icon's bitmap: BITMAPINFOHEADER, the color table of
// 1, 4 and 8 bit images, the bottom-up color rows and the 1 bit mask that
// makes pixels transparent. 32 bit images carry their own alpha; when every
// alpha is zero the mask applies, as Windows does.
func dibImage(data []byte) (image.Image, error) {
	invalid := errors.New("invalid icon bitmap")
	if len(data) < 40 {
		return nil, invalid
	}
	header := int(binary.LittleEndian.Uint32(data[0:4]))
	width := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	height := int(int32(binary.LittleEndian.Uint32(data[8:12]))) / 2
	bits := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := binary.LittleEndian.Uint32(data[16:20])
	used := int(binary.LittleEndian.Uint32(data[32:36]))
	if header < 40 || header > len(data) || width <= 0 || height <= 0 || width > 1024 || height > 1024 ||
		compression != 0 && !(compression == 3 && bits == 32) || !slices.Contains([]int{1, 4, 8, 24, 32}, bits) {
		return nil, invalid
	}
	offset := header
	var palette []color.NRGBA
	if bits <= 8 {
		colors := 1 << bits
		if used > 0 && used < colors {
			colors = used
		}
		if offset+4*colors > len(data) {
			return nil, invalid
		}
		for i := range colors {
			p := data[offset+4*i:]
			palette = append(palette, color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255})
		}
		offset += 4 * colors
	} else if compression == 3 && header == 40 {
		// BI_BITFIELDS with a bare header stores its three masks after it.
		offset += 12
	}
	stride := (width*bits + 31) / 32 * 4
	maskStride := (width + 31) / 32 * 4
	if offset+stride*height > len(data) {
		return nil, invalid
	}
	colorRows := data[offset : offset+stride*height]
	var mask []byte
	if end := offset + stride*height + maskStride*height; end <= len(data) {
		mask = data[offset+stride*height : end]
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	anyAlpha := false
	for y := range height {
		row := colorRows[(height-1-y)*stride:]
		for x := range width {
			var pixel color.NRGBA
			switch bits {
			case 32:
				pixel = color.NRGBA{R: row[4*x+2], G: row[4*x+1], B: row[4*x], A: row[4*x+3]}
				anyAlpha = anyAlpha || pixel.A != 0
			case 24:
				pixel = color.NRGBA{R: row[3*x+2], G: row[3*x+1], B: row[3*x], A: 255}
			default:
				perByte := 8 / bits
				index := int(row[x/perByte]>>(8-bits*(x%perByte+1))) & (1<<bits - 1)
				if index >= len(palette) {
					return nil, invalid
				}
				pixel = palette[index]
			}
			out.SetNRGBA(x, y, pixel)
		}
	}
	if bits == 32 && anyAlpha || mask == nil {
		if bits == 32 && !anyAlpha {
			// No alpha and no mask: the picture is opaque.
			for i := 3; i < len(out.Pix); i += 4 {
				out.Pix[i] = 255
			}
		}
		return out, nil
	}
	for y := range height {
		row := mask[(height-1-y)*maskStride:]
		for x := range width {
			transparent := row[x/8]>>(7-x%8)&1 == 1
			i := out.PixOffset(x, y) + 3
			switch {
			case transparent:
				out.Pix[i] = 0
			case bits == 32:
				out.Pix[i] = 255
			}
		}
	}
	return out, nil
}

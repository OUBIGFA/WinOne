package resultui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/egoist/mygo/ui"
	_ "github.com/fyne-io/image/ico"
	"image"
	"winone/internal/ai"
)

type providerAvatar struct {
	bitmap  *ui.Bitmap
	svg     *ui.SVG
	loading bool
	error   string
	bytes   int
	used    uint64
}

// A failed transfer is retryable after an explicit services refresh. Positive
// entries are immutable content hashes and never substitute another picture.
func (p *Panel) retryProviderAvatars() {
	for name, entry := range p.avatars {
		if !entry.loading && entry.error != "" {
			delete(p.avatars, name)
		}
	}
}

// Artwork follows current provider preferences. Preserve the historical name
// and model while avoiding a stale request-time icon after an avatar change.
func (p *Panel) messageProviderIcon(message *Message) string {
	if message.Provider != "" {
		if provider, ok := p.Configuration.Provider(message.Provider); ok {
			return provider.DisplayIcon()
		}
	}
	return message.ProviderIcon
}

// Decode with the shared native image loader (including photo orientation).
// Fyne supplies ICO support; its decoder assumes valid directory records, so
// bound and select one entry before it can allocate from an untrusted length.
func decodeProviderAvatar(data []byte) (avatar *providerAvatar, err error) {
	invalid := errors.New("头像图片无法解码，请重新选择图片")
	defer func() {
		if recover() != nil {
			avatar = nil
			err = invalid
		}
	}()
	if len(data) == 0 || len(data) > ai.MaxAvatarBytes {
		return nil, invalid
	}
	if bytes.Contains(bytes.ToLower(data[:min(1024, len(data))]), []byte("<svg")) {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return nil, invalid
		}
		return &providerAvatar{svg: svg, bytes: len(data)}, nil
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0, 0, 1, 0}) {
		data, err = singleICOImage(data)
		if err != nil {
			return nil, invalid
		}
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return nil, invalid
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, invalid
	}
	w, h := bitmap.Size()
	return &providerAvatar{bitmap: bitmap, bytes: w * h * 4}, nil
}

func singleICOImage(data []byte) ([]byte, error) {
	invalid := errors.New("invalid icon directory")
	if len(data) < 6 {
		return nil, invalid
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 64 || len(data) < 6+16*count {
		return nil, invalid
	}
	var selected []byte
	best := -1
	var payload []byte
	for i := 0; i < count; i++ {
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
			selected = entry
			payload = data[offset : offset+size]
			best = score
		}
	}
	result := make([]byte, 22+len(payload))
	copy(result, data[:6])
	binary.LittleEndian.PutUint16(result[4:], 1)
	copy(result[6:], selected)
	binary.LittleEndian.PutUint32(result[18:], 22)
	copy(result[22:], payload)
	return result, nil
}

func (p *Panel) providerImage(c *ui.Context, t *ui.Theme, name string, size float32) *ui.Element {
	if !ai.IsStoredAvatar(name) {
		return providerImage(c, t, name, size)
	}
	if p.avatars == nil {
		p.avatars = map[string]*providerAvatar{}
	}
	entry := p.avatars[name]
	if entry == nil {
		entry = &providerAvatar{loading: true}
		p.avatars[name] = entry
		if p.Actions.loadAvatar == nil {
			entry.loading = false
			entry.error = p.T("头像服务不可用", "Avatar service unavailable")
		} else {
			p.Actions.loadAvatar(name, func(value *providerAvatar, err error) {
				if p.avatars[name] != entry {
					return
				}
				if err != nil {
					entry.loading = false
					entry.error = err.Error()
					return
				}
				*entry = *value
				// Pictures are read once, separately from streamed text. Bound the
				// native cache and evict the least recently displayed old picture.
				for {
					total := 0
					oldestName := ""
					var oldest *providerAvatar
					for key, item := range p.avatars {
						total += item.bytes
						if key != name && !item.loading && (oldest == nil || item.used < oldest.used) {
							oldestName = key
							oldest = item
						}
					}
					if (total <= 64<<20 && len(p.avatars) <= 64) || oldest == nil {
						break
					}
					delete(p.avatars, oldestName)
				}
			})
		}
	}
	p.avatarClock++
	entry.used = p.avatarClock
	if entry.bitmap != nil {
		return ui.Image(c, entry.bitmap).Size(size, size).Fit(ui.Contain).Shrink(0).Role(ui.RoleNone)
	}
	if entry.svg != nil {
		// Standalone SVG images use black as their default currentColor, just
		// like the same image in an HTML img. The app theme must not tint it.
		return ui.Image(c, entry.svg).Size(size, size).Fit(ui.Contain).Shrink(0).TextColor(ui.Hex("#000000")).Role(ui.RoleNone)
	}
	element := providerImage(c, t, "service", size)
	if entry.error != "" {
		element.Tooltip(entry.error)
	}
	return element
}

package onebot

import (
	"encoding/base64"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit/icons"
)

type avatarEntry struct {
	image   *ui.Bitmap
	loading bool
	error   string
}
type avatarCache struct{ entries map[string]*avatarEntry }

func (a *avatarCache) load(p *page, profiles []Profile) {
	if a.entries == nil {
		a.entries = map[string]*avatarEntry{}
	}
	for _, bot := range profiles {
		address := bot.Avatar
		if !strings.HasPrefix(address, "cindy-media://blobs/") {
			continue
		}
		if _, exists := a.entries[address]; exists {
			continue
		}
		entry := &avatarEntry{loading: true}
		a.entries[address] = entry
		oneBotCall(p, "media.read", map[string]string{"url": address}, func(value struct {
			Base64 string `json:"base64"`
		}, err error) {
			entry.loading = false
			if err != nil {
				entry.error = err.Error()
				return
			}
			data, err := base64.StdEncoding.DecodeString(value.Base64)
			if err != nil {
				entry.error = nt("avatarFailed")
				return
			}
			entry.image, err = ui.DecodeBitmap(data)
			if err != nil {
				entry.error = nt("avatarFailed")
			}
		})
	}
}
func (a *avatarCache) View(c *ui.Context, bot Profile, size float32) {
	box := ui.Box(c).Width(size).Height(size).Shrink(0).Radius(size / 2).Clip().Label(bot.Name)
	box.Children(func() {
		if entry := a.entries[bot.Avatar]; entry != nil {
			if entry.image != nil {
				ui.Image(c, entry.image).Fill().Fit(ui.Contain).Label(bot.Name)
				return
			}
			if entry.error != "" {
				ui.Icon(c, icons.ImageOff).FontSize(size * .6).Tooltip(entry.error)
				return
			}
		}
		if bot.Avatar != "" && !strings.Contains(bot.Avatar, "://") && !strings.HasPrefix(bot.Avatar, "data:") && bot.Avatar != "default" {
			ui.Text(c, bot.Avatar).FontSize(size * .65).Label(bot.Name)
		} else {
			ui.Icon(c, icons.Panda).FontSize(size * .7).Label(bot.Name)
		}
	})
}

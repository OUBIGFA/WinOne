package interconnect

import (
	"encoding/base64"
	"math"
	"strconv"
	"net/url"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// qrCache keeps decoded QR codes by their source text: the services send
// the same picture on every poll.
var qrCache sync.Map

// qrSource decodes a QR code the transfer services give: a data URL of a
// picture, or SVG text. It returns nil for anything else.
func qrSource(value string) ui.ImageSource {
	if value == "" {
		return nil
	}
	if cached, ok := qrCache.Load(value); ok {
		return cached.(ui.ImageSource)
	}
	var data []byte
	switch {
	case strings.HasPrefix(value, "data:"):
		header, body, ok := strings.Cut(value, ",")
		if !ok {
			return nil
		}
		if strings.HasSuffix(header, ";base64") {
			decoded, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				return nil
			}
			data = decoded
		} else {
			decoded, err := url.PathUnescape(body)
			if err != nil {
				return nil
			}
			data = []byte(decoded)
		}
	case strings.Contains(value, "<svg"):
		data = []byte(value)
	default:
		return nil
	}
	source, err := ui.DecodeImage(data)
	if err != nil {
		return nil
	}
	qrCache.Store(value, source)
	return source
}

// qrImage shows a QR code on white, as scanners need, at a fixed size.
func qrImage(c *ui.Context, value, label string) {
	source := qrSource(value)
	if source == nil {
		return
	}
	pal := kit.Colors(c)
	ui.Box(c).Size(208, 208).Padding(8).Radius(kit.PanelRadius).Background(ui.RGB(255, 255, 255)).Border(1, pal.Hairline).
		Children(func() {
			ui.Image(c, source).Fill().Fit(ui.Contain).Label(label).Role(ui.RoleImage)
		})
}

// size formats bytes as KiB or MiB with at most one decimal and grouped
// thousands, as the React page's Intl.NumberFormat did.
func size(n float64) string {
	unit, value := " KiB", n/1024
	if n >= 1048576 {
		unit, value = " MiB", n/1048576
	}
	text := strconv.FormatFloat(math.Round(value*10)/10, 'f', -1, 64)
	whole, fraction, _ := strings.Cut(text, ".")
	for i := len(whole) - 3; i > 0 && whole[i-1] != '-'; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if fraction != "" {
		whole += "." + fraction
	}
	return whole + unit
}

// tr reads an interconnect string.
func tr(key string, args ...any) string { return i18n.T("interconnect."+key, args...) }

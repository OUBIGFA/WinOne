package worker

import (
	"bytes"
	"image"
	"image/png"
)

// cachedPNG returns the PNG encoding of img for the Host's tray menu. The tray
// box replaces an icon's *image.NRGBA whenever the program sets a new icon and
// never changes a published image in place, so the pointer identifies its
// pixels. previous is the last poll's cache; next collects the encodings still
// shown, so images of removed or changed icons are released.
func cachedPNG(previous, next map[*image.NRGBA][]byte, img *image.NRGBA) ([]byte, error) {
	if img == nil {
		return nil, nil
	}
	if data, ok := next[img]; ok {
		return data, nil
	}
	data, ok := previous[img]
	if !ok {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, img); err != nil {
			return nil, err
		}
		data = buffer.Bytes()
	}
	next[img] = data
	return data, nil
}

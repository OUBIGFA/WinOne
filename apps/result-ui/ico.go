package resultui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"slices"
)

// Windows icon files are a picture source of custom provider avatars
// (Magpie's favicon and file pickers). Neither the standard library nor
// MyGo's DecodeBitmap reads them, so the largest image is decoded here: a
// PNG, or a device independent bitmap with its transparency mask. This is
// the algorithm of apps/desktop-mygo/widgets/provider_icon.go, so both
// windows show the same picture; it replaces fyne-io/image, whose ICO
// reader treated the 0 size byte (256 pixels) as empty and needed a build
// patch.

// icoImage decodes the largest image of an icon file. Every directory entry
// is bounded by the file before anything is allocated from its claims.
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
		// A size byte of 0 means 256 pixels.
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
		config, _, err := image.DecodeConfig(bytes.NewReader(payload))
		if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 {
			return nil, invalid
		}
		picture, _, err := image.Decode(bytes.NewReader(payload))
		if err != nil {
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

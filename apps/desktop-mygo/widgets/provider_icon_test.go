package widgets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// icoFile wraps payloads as the entries of an icon file.
func icoFile(t *testing.T, sizes []int, payloads ...[]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(payloads))})
	offset := 6 + 16*len(payloads)
	for i, payload := range payloads {
		size := byte(sizes[i] % 256)
		binary.Write(&out, binary.LittleEndian, [4]byte{size, size, 0, 0})
		binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(payload)), uint32(offset)})
		offset += len(payload)
	}
	for _, payload := range payloads {
		out.Write(payload)
	}
	return out.Bytes()
}

// dib builds an icon bitmap of 2×2 pixels: rows bottom-up, then the mask.
func dib(bits int, palette []color.NRGBA, rows [][]byte, mask [][]byte) []byte {
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, struct {
		Size          uint32
		Width, Height int32
		Planes, Bits  uint16
		Compression   uint32
		Image         uint32
		X, Y          int32
		Used, Needed  uint32
	}{Size: 40, Width: 2, Height: 4, Planes: 1, Bits: uint16(bits), Used: uint32(len(palette))})
	for _, c := range palette {
		out.Write([]byte{c.B, c.G, c.R, 0})
	}
	for _, row := range rows {
		out.Write(row)
	}
	for _, row := range mask {
		out.Write(row)
	}
	return out.Bytes()
}

func TestIconFilesDecodeWithTheirTransparency(t *testing.T) {
	opaque := [][]byte{{0, 0, 0, 0}, {0, 0, 0, 0}}
	// Bottom row first: the lower left pixel is red, the upper right blue.
	bgra := [][]byte{{0, 0, 255, 255, 0, 0, 0, 0}, {0, 0, 0, 0, 255, 0, 0, 128}}
	picture, err := icoImage(icoFile(t, []int{2}, dib(32, nil, bgra, opaque)))
	if err != nil {
		t.Fatal(err)
	}
	if got := picture.At(0, 1); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("lower left = %v", got)
	}
	if got := picture.At(1, 0); got != (color.NRGBA{B: 255, A: 128}) {
		t.Fatalf("upper right keeps its alpha: %v", got)
	}

	// Without any alpha the mask decides: the upper left is transparent.
	noAlpha := [][]byte{{0, 0, 255, 0, 0, 255, 0, 0}, {255, 0, 0, 0, 255, 255, 255, 0}}
	mask := [][]byte{{0, 0, 0, 0}, {0x80, 0, 0, 0}}
	picture, err = icoImage(icoFile(t, []int{2}, dib(32, nil, noAlpha, mask)))
	if err != nil {
		t.Fatal(err)
	}
	if got := picture.At(0, 0).(color.NRGBA); got.A != 0 {
		t.Fatalf("masked pixel shows: %v", got)
	}
	if got := picture.At(1, 1); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("unmasked pixel = %v", got)
	}

	// A palette image: index 1 is green, the mask hides the lower right.
	palette := []color.NRGBA{{A: 255}, {G: 200, A: 255}}
	indexed := [][]byte{{0x01, 0, 0, 0}, {0x01, 0x01, 0, 0}}
	paletteMask := [][]byte{{0x40, 0, 0, 0}, {0, 0, 0, 0}}
	picture, err = icoImage(icoFile(t, []int{2}, dib(8, palette, indexed, paletteMask)))
	if err != nil {
		t.Fatal(err)
	}
	if got := picture.At(0, 1); got != (color.NRGBA{G: 200, A: 255}) {
		t.Fatalf("palette pixel = %v", got)
	}
	if got := picture.At(1, 1).(color.NRGBA); got.A != 0 {
		t.Fatalf("masked palette pixel shows: %v", got)
	}
}

func TestIconFilesUseTheirLargestImage(t *testing.T) {
	var encoded bytes.Buffer
	large := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	large.SetNRGBA(2, 2, color.NRGBA{R: 9, A: 255})
	if err := png.Encode(&encoded, large); err != nil {
		t.Fatal(err)
	}
	small := dib(32, nil, [][]byte{make([]byte, 8), make([]byte, 8)}, nil)
	picture, err := icoImage(icoFile(t, []int{2, 3}, small, encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != 3 || picture.At(2, 2).(color.NRGBA).R != 9 {
		t.Fatalf("picked %v", picture.Bounds())
	}
	if _, err := icoImage(icoFile(t, []int{2}, []byte("short"))); err == nil {
		t.Fatal("a broken entry decodes")
	}
	if _, err := decodeAvatar([]byte{0, 0, 1, 0, 9, 9}); err == nil {
		t.Fatal("a broken icon file decodes")
	}
}

func TestBuiltInProviderIconsPairColorVariants(t *testing.T) {
	names := BuiltInProviderIcons()
	kimi, kimiColor := slices.Index(names, "kimi"), slices.Index(names, "kimi-color")
	if kimi < 0 || kimiColor < 0 || kimiColor != kimi-1 {
		t.Fatalf("kimi variants at %d and %d", kimiColor, kimi)
	}
	if !slices.Contains(names, "zcode") || slices.Contains(names, "zcode.png") {
		t.Fatal("pictures are listed by name")
	}
	light := &ui.Theme{Text: ui.Color{A: 255}}
	if builtInProviderBitmap("openai", light) == nil || builtInProviderBitmap("zcode", light) == nil {
		t.Fatal("built-in marks have no picture")
	}
	if builtInProviderBitmap("unknown", light) != nil || builtInProviderBitmap("../openai", light) != nil {
		t.Fatal("unknown names have a picture")
	}
}

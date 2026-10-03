// Command icon derives Windows icons from the shared project logo.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	source := flag.String("source", "internal/branding/logo.png", "source PNG")
	output := flag.String("out", "internal/branding/icon.ico", "output icon")
	pngDirectory := flag.String("png-dir", "", "optional directory for standard PNG sizes")
	nativePNG := flag.String("native-png", "", "optional 64-pixel native menu logo")
	flag.Parse()
	f, err := os.Open(*source)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		panic(err)
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}
	data := make([]byte, 6+16*len(sizes))
	binary.LittleEndian.PutUint16(data[2:], 1)
	binary.LittleEndian.PutUint16(data[4:], uint16(len(sizes)))
	for i, size := range sizes {
		var payload bytes.Buffer
		if err := png.Encode(&payload, resize(img, size)); err != nil {
			panic(err)
		}
		entry := data[6+i*16 : 6+(i+1)*16]
		entry[0], entry[1] = byte(size), byte(size) // ICO encodes 256 as zero.
		binary.LittleEndian.PutUint16(entry[4:], 1)
		binary.LittleEndian.PutUint16(entry[6:], 32)
		binary.LittleEndian.PutUint32(entry[8:], uint32(payload.Len()))
		binary.LittleEndian.PutUint32(entry[12:], uint32(len(data)))
		data = append(data, payload.Bytes()...)
	}
	if err := os.WriteFile(*output, data, 0644); err != nil {
		panic(err)
	}
	if *pngDirectory != "" {
		if err := os.MkdirAll(*pngDirectory, 0755); err != nil {
			panic(err)
		}
		for _, size := range append(sizes, 512, 1024) {
			var payload bytes.Buffer
			if err := png.Encode(&payload, resize(img, size)); err != nil {
				panic(err)
			}
			if err := os.WriteFile(filepath.Join(*pngDirectory, fmt.Sprintf("winone-%d.png", size)), payload.Bytes(), 0644); err != nil {
				panic(err)
			}
		}
	}
	if *nativePNG != "" {
		var payload bytes.Buffer
		if err := png.Encode(&payload, resize(img, 64)); err != nil {
			panic(err)
		}
		if err := os.WriteFile(*nativePNG, payload.Bytes(), 0644); err != nil {
			panic(err)
		}
	}
}

// Average premultiplied pixels to keep small edges smooth; fit without stretching.
func resize(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	w, h := size, size
	if b.Dx() > b.Dy() {
		h = max(1, size*b.Dy()/b.Dx())
	} else {
		w = max(1, size*b.Dx()/b.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, blue, a, count uint64
			for sy := y * b.Dy() / h; sy < max(y*b.Dy()/h+1, (y+1)*b.Dy()/h); sy++ {
				for sx := x * b.Dx() / w; sx < max(x*b.Dx()/w+1, (x+1)*b.Dx()/w); sx++ {
					pr, pg, pb, pa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					blue += uint64(pb)
					a += uint64(pa)
					count++
				}
			}
			dst.SetRGBA(x+(size-w)/2, y+(size-h)/2, color.RGBA{uint8(r / count >> 8), uint8(g / count >> 8), uint8(blue / count >> 8), uint8(a / count >> 8)})
		}
	}
	return dst
}

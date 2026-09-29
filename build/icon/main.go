// Command icon creates WinOne's small monochrome tray icon without external tools.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	output := flag.String("out", "internal/branding/icon.ico", "output icon")
	flag.Parse()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 2; y < 30; y++ {
		for x := 2; x < 30; x++ {
			img.Set(x, y, color.NRGBA{R: 40, G: 40, B: 40, A: 255})
		}
	}
	// Draw a pixel-aligned W so the tray remains legible at small sizes.
	rows := []string{"1000001", "1000001", "1001001", "1001001", "1010101", "1100011", "1000001"}
	for y, row := range rows {
		for x, bit := range row {
			if bit == '1' {
				for dy := 0; dy < 2; dy++ {
					for dx := 0; dx < 2; dx++ {
						img.Set(9+x*2+dx, 9+y*2+dy, color.White)
					}
				}
			}
		}
	}
	var payload bytes.Buffer
	if err := png.Encode(&payload, img); err != nil {
		panic(err)
	}
	data := make([]byte, 22)
	binary.LittleEndian.PutUint16(data[2:], 1)
	binary.LittleEndian.PutUint16(data[4:], 1)
	data[6] = 32
	data[7] = 32
	binary.LittleEndian.PutUint16(data[10:], 1)
	binary.LittleEndian.PutUint16(data[12:], 32)
	binary.LittleEndian.PutUint32(data[14:], uint32(payload.Len()))
	binary.LittleEndian.PutUint32(data[18:], 22)
	if err := os.WriteFile(*output, append(data, payload.Bytes()...), 0644); err != nil {
		panic(err)
	}
}

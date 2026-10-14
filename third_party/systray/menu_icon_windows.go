//go:build windows

package systray

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

// Keep source pixels until the popup knows its monitor DPI. All DIBs use
// premultiplied alpha; resampling straight alpha would darken transparent edges.
func rgbaMenuBitmap(source image.Image) (windows.Handle, error) {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	bitmap, err := create32BitHBitmap(0, int32(width), -int32(height))
	if err != nil {
		return 0, err
	}
	var info nativeMenuBitmap
	if ok, _, _ := menuBitmapInfo.Call(bitmap, unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); ok == 0 || info.Bits == 0 {
		menuDeleteObject.Call(bitmap)
		return 0, fmt.Errorf("menu bitmap pixels unavailable")
	}
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(info.Bits)), width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			offset := (y*width + x) * 4
			pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = byte(b>>8), byte(g>>8), byte(r>>8), byte(a>>8)
		}
	}
	return windows.Handle(bitmap), nil
}

func scaledMenuBitmap(bitmap windows.Handle, size int32) (windows.Handle, error) {
	var info nativeMenuBitmap
	if ok, _, _ := menuBitmapInfo.Call(uintptr(bitmap), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); ok == 0 || info.Width <= 0 || info.Height <= 0 || info.Width > 256 || info.Height > 256 {
		return 0, fmt.Errorf("invalid menu bitmap dimensions")
	}
	source := image.NewRGBA(image.Rect(0, 0, int(info.Width), int(info.Height)))
	header := bitmapInfo{BmiHeader: bitmapInfoHeader{BiWidth: info.Width, BiHeight: -info.Height, BiPlanes: 1, BiBitCount: 32}}
	header.BmiHeader.BiSize = uint32(unsafe.Sizeof(header.BmiHeader))
	dc, _, err := pCreateCompatibleDC.Call(0)
	if dc == 0 {
		return 0, err
	}
	defer pDeleteDC.Call(dc)
	rows, _, err := g32.NewProc("GetDIBits").Call(dc, uintptr(bitmap), 0, uintptr(info.Height), uintptr(unsafe.Pointer(&source.Pix[0])), uintptr(unsafe.Pointer(&header)), 0)
	runtime.KeepAlive(source)
	if rows != uintptr(info.Height) {
		return 0, fmt.Errorf("read menu bitmap: %v", err)
	}
	for i := 0; i < len(source.Pix); i += 4 {
		source.Pix[i], source.Pix[i+2] = source.Pix[i+2], source.Pix[i]
	}
	destination := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	// The standard Go image scaler filters both upscaling and fractional DPI
	// downscaling. AlphaBlend then only composites equal-sized pixels.
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	return rgbaMenuBitmap(destination)
}

//go:build webui

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

// printWindow saves the window as the compositor draws it, web views
// included (PW_RENDERFULLCONTENT), to check where the browser tab's page
// shows over the panel.
func printWindow(hwnd uintptr, path string) error {
	var r struct{ left, top, right, bottom int32 }
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.right-r.left), int(r.bottom-r.top)
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(dc)
	header := struct {
		size                        uint32
		width, height               int32
		planes, bits                uint16
		compression, image          uint32
		xppm, yppm, used, important int32
	}{size: 40, width: int32(w), height: int32(-h), planes: 1, bits: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return windows.GetLastError()
	}
	defer procDeleteObject.Call(bitmap)
	procSelectObject.Call(dc, bitmap)
	const clientOnly, fullContent = 1, 2
	if ok, _, err := procPrintWindow.Call(hwnd, dc, clientOnly|fullContent); ok == 0 {
		return err
	}
	pixels := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.SetNRGBA(i%w, i/w, color.NRGBA{pixels[i*4+2], pixels[i*4+1], pixels[i*4], 255})
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

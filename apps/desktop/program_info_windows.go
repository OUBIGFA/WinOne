//go:build windows

package main

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/windows"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

type ProgramMetadata struct {
	Name string `json:"name"`
	Icon []byte `json:"icon,omitempty"`
}

// ProgramInfo reads file resources only. It neither starts the program nor
// changes its registration, so disabled modules can use the same picker.
func (a *App) ProgramInfo(path string) (ProgramMetadata, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initErr := windows.CoInitializeEx(0, 2)
	if initErr != nil && initErr != syscall.Errno(1) && initErr != syscall.Errno(0x80010106) {
		return ProgramMetadata{}, initErr
	}
	// S_FALSE adds a COM reference; RPC_E_CHANGED_MODE means the thread already
	// has another apartment and must not be uninitialized by this call.
	if initErr != syscall.Errno(0x80010106) {
		defer windows.CoUninitialize()
	}
	result := ProgramMetadata{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if info.IsDir() {
		return result, fmt.Errorf("not a program file")
	}
	if name := programDescription(path); name != "" {
		result.Name = name
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return result, err
	}
	var shell struct {
		Icon       uintptr
		Index      int32
		Attributes uint32
		Display    [260]uint16
		Type       [80]uint16
	}
	// Match Explorer/WinTray's file icon. An icon-less file legitimately uses
	// the generic UI icon; every returned HICON is owned and released here.
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	ok, _, _ := shell32.NewProc("SHGetFileInfoW").Call(uintptr(unsafe.Pointer(ptr)), 0, uintptr(unsafe.Pointer(&shell)), unsafe.Sizeof(shell), 0x100)
	if ok != 0 && shell.Icon != 0 {
		defer windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon").Call(shell.Icon)
		result.Icon, err = programIconPNG(shell.Icon)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func programDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 || size > 4<<20 {
		return ""
	}
	data := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])) != nil {
		return ""
	}
	defer runtime.KeepAlive(data)
	var translations *uint16
	var length uint32
	if windows.VerQueryValue(unsafe.Pointer(&data[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&translations), &length) != nil || translations == nil || length < 4 {
		return ""
	}
	pairs := unsafe.Slice(translations, int(length/2))
	for _, field := range []string{"FileDescription", "ProductName"} {
		for i := 0; i+1 < len(pairs); i += 2 {
			key := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, pairs[i], pairs[i+1], field)
			var value *uint16
			var count uint32
			if windows.VerQueryValue(unsafe.Pointer(&data[0]), key, unsafe.Pointer(&value), &count) == nil && value != nil && count > 0 && count < 4096 {
				if name := strings.TrimSpace(windows.UTF16ToString(unsafe.Slice(value, int(count)))); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

// Render on black and white to recover transparency for both modern alpha
// icons and legacy masked icons, as in WinTray's traybox bitmap conversion.
func programIconPNG(icon uintptr) ([]byte, error) {
	black, err := drawProgramIcon(icon, 0)
	if err != nil {
		return nil, err
	}
	white, err := drawProgramIcon(icon, 255)
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < len(black); i += 4 {
		difference := (int(white[i]) - int(black[i]) + int(white[i+1]) - int(black[i+1]) + int(white[i+2]) - int(black[i+2])) / 3
		alpha := min(255, max(0, 255-difference))
		if alpha == 0 {
			continue
		}
		for c := 0; c < 3; c++ {
			out.Pix[i+c] = byte(min(255, int(black[i+2-c])*255/alpha))
		}
		out.Pix[i+3] = byte(alpha)
	}
	var buffer bytes.Buffer
	err = png.Encode(&buffer, out)
	return buffer.Bytes(), err
}
func drawProgramIcon(icon uintptr, background byte) ([]byte, error) {
	gdi := windows.NewLazySystemDLL("gdi32.dll")
	dc, _, _ := gdi.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer gdi.NewProc("DeleteDC").Call(dc)
	header := struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		X, Y                   int32
		Used, Important        uint32
	}{Size: 40, Width: 32, Height: -32, Planes: 1, Bits: 32}
	var pixels unsafe.Pointer
	bitmap, _, _ := gdi.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return nil, fmt.Errorf("CreateDIBSection failed")
	}
	defer gdi.NewProc("DeleteObject").Call(bitmap)
	old, _, _ := gdi.NewProc("SelectObject").Call(dc, bitmap)
	defer gdi.NewProc("SelectObject").Call(dc, old)
	data := unsafe.Slice((*byte)(pixels), 32*32*4)
	for i := range data {
		data[i] = background
	}
	ok, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("DrawIconEx").Call(dc, 0, 0, icon, 32, 32, 0, 0, 3)
	if ok == 0 {
		return nil, fmt.Errorf("DrawIconEx failed")
	}
	gdi.NewProc("GdiFlush").Call()
	return append([]byte(nil), data...), nil
}

//go:build windows

// Package desktop holds what the settings window does itself rather than
// through the Host: reading a program's name and icon, running the Host's
// administrator helper for the permission settings, opening a folder in
// File Explorer and ending this window's process. The previous window
// provided these through its native bridge (apps/desktop-rust and the
// older apps/desktop); the behavior and limits are kept.
package desktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/internal/settings"
)

// ProgramMetadata is what File Explorer shows for a program: its
// description (or file name) and its icon as a 32×32 PNG, empty when the
// file has none.
type ProgramMetadata struct {
	Name string `json:"name"`
	Icon []byte `json:"icon,omitempty"`
}

// programSlots bounds Shell reads, so a long list or a large drop does not
// start a thread per program (React kept four reads at a time).
var programSlots = make(chan struct{}, 4)

// ProgramInfo reads path's name and icon. It only reads the file's
// resources: it neither starts the program nor changes its registration.
// With a profile directory, icons are cached in cache/program-icons by the
// file's size and modification time, as the previous window did. It
// blocks: call it off the UI thread.
func ProgramInfo(directory, path string) (ProgramMetadata, error) {
	programSlots <- struct{}{}
	defer func() { <-programSlots }()
	if directory == "" {
		return readProgramInfo(path)
	}
	return cachedProgramInfo(filepath.Join(directory, "cache", "program-icons"), path, readProgramInfo)
}

func readProgramInfo(path string) (ProgramMetadata, error) {
	// SHGetFileInfo needs COM on a thread of its own apartment.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initErr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	if initErr != nil && initErr != syscall.Errno(1) && initErr != syscall.Errno(0x80010106) {
		return ProgramMetadata{}, initErr
	}
	// S_FALSE adds a COM reference; RPC_E_CHANGED_MODE means the thread
	// already has another apartment, which this call must not end.
	if initErr != syscall.Errno(0x80010106) {
		defer windows.CoUninitialize()
	}
	result := ProgramMetadata{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if info.IsDir() {
		return result, core.Errorf(i18n.T("wintray.notProgramFile"))
	}
	if name := programDescription(path); name != "" {
		result.Name = name
	}
	pointer, err := windows.UTF16PtrFromString(path)
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
	// Match File Explorer's icon. A file without one keeps the generic
	// icon of the list; every returned HICON is released here.
	ok, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(pointer)), 0, uintptr(unsafe.Pointer(&shell)), unsafe.Sizeof(shell), shgfiIcon)
	if ok != 0 && shell.Icon != 0 {
		defer procDestroyIcon.Call(shell.Icon)
		result.Icon, err = programIconPNG(shell.Icon)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

const shgfiIcon = 0x100

var (
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procSHGetFileInfoW     = shell32.NewProc("SHGetFileInfoW")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procDrawIconEx         = user32.NewProc("DrawIconEx")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procGdiFlush           = gdi32.NewProc("GdiFlush")
)

// programDescription returns the file's FileDescription, else its
// ProductName, in the first language the file lists.
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

// programIconPNG draws the icon on black and on white to recover its
// transparency, for modern alpha icons and legacy masked ones alike, as
// WinTray's own bitmap conversion does.
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
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(dc)
	header := struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		X, Y                   int32
		Used, Important        uint32
	}{Size: 40, Width: 32, Height: -32, Planes: 1, Bits: 32}
	var pixels unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return nil, fmt.Errorf("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(bitmap)
	old, _, _ := procSelectObject.Call(dc, bitmap)
	defer procSelectObject.Call(dc, old)
	data := unsafe.Slice((*byte)(pixels), 32*32*4)
	for i := range data {
		data[i] = background
	}
	if ok, _, _ := procDrawIconEx.Call(dc, 0, 0, icon, 32, 32, 0, 0, 3); ok == 0 { // DI_NORMAL
		return nil, fmt.Errorf("DrawIconEx failed")
	}
	procGdiFlush.Call()
	return append([]byte(nil), data...), nil
}

type programCacheEntry struct {
	Version  int
	Size     int64
	Modified int64
	Metadata ProgramMetadata
}

var programCacheMu sync.Mutex

// cachedProgramInfo keeps extracted icons across sessions, keyed by the
// path and invalidated by the file's size and write time. A cache that
// cannot be written is no reason to fail the read.
func cachedProgramInfo(directory, path string, read func(string) (ProgramMetadata, error)) (ProgramMetadata, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ProgramMetadata{}, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return ProgramMetadata{}, err
	}
	if stat.IsDir() {
		return ProgramMetadata{}, core.Errorf(i18n.T("wintray.notProgramFile"))
	}
	key := fmt.Sprintf("%x.json", sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute)))))
	file := filepath.Join(directory, key)
	programCacheMu.Lock()
	defer programCacheMu.Unlock()
	if data, err := os.ReadFile(file); err == nil && len(data) <= 1<<20 {
		var entry programCacheEntry
		if json.Unmarshal(data, &entry) == nil && entry.Version == 1 && entry.Size == stat.Size() && entry.Modified == stat.ModTime().UnixNano() && len(entry.Metadata.Icon) > 0 {
			return entry.Metadata, nil
		}
	}
	value, err := read(absolute)
	if err != nil || len(value.Icon) == 0 {
		return value, err
	}
	entry := programCacheEntry{1, stat.Size(), stat.ModTime().UnixNano(), value}
	if data, err := json.Marshal(entry); err == nil {
		if err = settings.WriteAtomic(file, data); err != nil {
			fmt.Fprintln(os.Stderr, "program icon cache:", err)
		}
	}
	return value, nil
}

//go:build windows

package ai

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"image/png"
	"strings"
	"unsafe"
)

type ImageReference struct {
	Mapping string `json:"mapping"`
	Bytes   int    `json:"bytes"`
}

// Modules transfer a PNG once through an owned memory mapping. The Host takes
// an immutable snapshot before acknowledging the job; polling carries no image.
func ReadImage(ref ImageReference, peer uint32) ([]byte, error) {
	prefix := fmt.Sprintf(`Local\WinOne.AI.%d.`, peer)
	if peer == 0 || ref.Bytes < 1 || ref.Bytes > 32<<20 || !strings.HasPrefix(ref.Mapping, prefix) {
		return nil, errors.New("AI 图片引用无效")
	}
	suffix := strings.TrimPrefix(ref.Mapping, prefix)
	if decoded, err := hex.DecodeString(suffix); err != nil || len(decoded) != 16 {
		return nil, errors.New("AI 图片引用无效")
	}
	name, err := windows.UTF16PtrFromString(ref.Mapping)
	if err != nil {
		return nil, err
	}
	handle, _, openErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenFileMappingW").Call(windows.FILE_MAP_READ, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, fmt.Errorf("AI 图片无法读取：%w", openErr)
	}
	defer windows.CloseHandle(windows.Handle(handle))
	view, err := windows.MapViewOfFile(windows.Handle(handle), windows.FILE_MAP_READ, 0, 0, uintptr(ref.Bytes))
	if err != nil {
		return nil, errors.New("AI 图片大小与引用不匹配")
	}
	defer windows.UnmapViewOfFile(view)
	// A mapped address is not Go-managed memory. Copy through the Windows API
	// instead of converting its uintptr to an unsafe Go slice.
	snapshot := make([]byte, ref.Bytes)
	var copied uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), view, &snapshot[0], uintptr(len(snapshot)), &copied); err != nil || copied != uintptr(len(snapshot)) {
		return nil, errors.New("AI 图片读取不完整")
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(snapshot))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 || int64(configuration.Width)*int64(configuration.Height) > 80_000_000 {
		return nil, errors.New("AI 图片格式无效或尺寸过大")
	}
	return snapshot, nil
}

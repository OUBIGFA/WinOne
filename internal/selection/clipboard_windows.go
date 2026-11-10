//go:build windows

package selection

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only the reader's verified text-control outcome admits this path. Keep the
// transaction in the Host: cancellation can kill the UIA worker, but must not
// kill the owner responsible for restoring the user's clipboard.
const copyInputMarker = uintptr(0x574F5345)

var clipboardSlot = make(chan struct{}, 1)
var physicalCopyRevision atomic.Uint64
var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var gdi32 = windows.NewLazySystemDLL("gdi32.dll")

type clipboardFormat struct {
	format uint32
	data   []byte
	bitmap uintptr
}

func cloneClipboardBitmap(handle uintptr) uintptr {
	copy, _, _ := user32.NewProc("CopyImage").Call(handle, 0, 0, 0, 0)
	return copy
}

func freeClipboardHandle(format uint32, handle uintptr) {
	if handle == 0 {
		return
	}
	if format == 2 {
		gdi32.NewProc("DeleteObject").Call(handle)
	} else {
		kernel32.NewProc("GlobalFree").Call(handle)
	}
}

func clipboardSequence() uintptr {
	n, _, _ := user32.NewProc("GetClipboardSequenceNumber").Call()
	return n
}

func openClipboard(deadline time.Time) error {
	return openClipboardFor(0, deadline)
}

func openClipboardFor(owner uintptr, deadline time.Time) error {
	for {
		if ok, _, _ := user32.NewProc("OpenClipboard").Call(owner); ok != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("剪贴板正被其他程序占用")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func clipboardBytes(format uint32, limit uintptr) ([]byte, error) {
	handle, _, _ := user32.NewProc("GetClipboardData").Call(uintptr(format))
	if handle == 0 {
		return nil, errors.New("无法读取剪贴板格式")
	}
	size, _, _ := kernel32.NewProc("GlobalSize").Call(handle)
	if size == 0 || size > limit {
		return nil, errors.New("剪贴板格式无法安全保存")
	}
	ptr, _, _ := kernel32.NewProc("GlobalLock").Call(handle)
	if ptr == 0 {
		return nil, errors.New("无法锁定剪贴板格式")
	}
	defer kernel32.NewProc("GlobalUnlock").Call(handle)
	data := make([]byte, int(size))
	kernel32.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&data[0])), ptr, size)
	return data, nil
}

// GDI/metafile/owner-display formats are handles rather than global-memory
// bytes. Refuse the compatibility copy instead of losing any of those formats.
func memoryClipboardFormat(format uint32) bool {
	return (format >= 1 && format <= 17 && format != 2 && format != 3 && format != 9 && format != 14) || format >= 0xC000
}

// OLE's transport descriptors refer to the previous owner/data object. They
// are not user payloads and cannot be replayed after replacing that owner.
func clipboardTransportFormat(name string) bool {
	return name == "DataObject" || name == "Ole Private Data"
}

func snapshotClipboard() ([]clipboardFormat, uintptr, error) {
	if err := openClipboard(time.Now().Add(60 * time.Millisecond)); err != nil {
		return nil, 0, err
	}
	defer user32.NewProc("CloseClipboard").Call()
	var saved []clipboardFormat
	complete := false
	defer func() {
		if !complete {
			for _, item := range saved {
				freeClipboardHandle(2, item.bitmap)
			}
		}
	}()
	budget := uintptr(64 << 20)
	var imageFormat uintptr
	for format := uintptr(0); ; {
		format, _, _ = user32.NewProc("EnumClipboardFormats").Call(format)
		if format == 0 {
			break
		}
		var name [128]uint16
		user32.NewProc("GetClipboardFormatNameW").Call(format, uintptr(unsafe.Pointer(&name[0])), 128)
		if clipboardTransportFormat(windows.UTF16ToString(name[:])) {
			continue
		}
		// EnumClipboardFormats lists originals before Windows-generated
		// conversions. Preserve the original image representation only; setting
		// a synthesized DIB later can replace a perfectly valid bitmap.
		if format == 2 || format == 8 || format == 17 {
			if imageFormat != 0 {
				continue
			}
			imageFormat = format
		}
		if format == 2 {
			// CF_BITMAP is a GDI handle, not bytes. Preserve an independent
			// bitmap: recreating it from synthesized DIBs can change pixels.
			handle, _, _ := user32.NewProc("GetClipboardData").Call(format)
			bitmap := cloneClipboardBitmap(handle)
			if bitmap == 0 {
				return nil, 0, errors.New("无法保存剪贴板图片")
			}
			saved = append(saved, clipboardFormat{format: 2, bitmap: bitmap})
			continue
		}
		if !memoryClipboardFormat(uint32(format)) {
			return nil, 0, errors.New("剪贴板含不支持保留的格式，已跳过兼容复制")
		}
		data, err := clipboardBytes(uint32(format), budget)
		if err != nil {
			return nil, 0, err
		}
		budget -= uintptr(len(data))
		if os.Getenv("WINONE_SELECTION_DIAGNOSTICS") == "1" {
			fmt.Fprintf(os.Stderr, "selection clipboard format: id=%d name=%s bytes=%d\n", format, windows.UTF16ToString(name[:]), len(data))
		}
		saved = append(saved, clipboardFormat{format: uint32(format), data: data})
	}
	complete = true
	return saved, clipboardSequence(), nil
}

// The caller holds the same clipboard lock used to read the fresh text.
// Releasing it between read and restore allows viewers and delayed formats to
// change the sequence and made restoration spuriously skip valid snapshots.
func restoreClipboardLocked(saved []clipboardFormat) error {
	handles := make([]uintptr, len(saved))
	defer func() {
		for i, h := range handles {
			freeClipboardHandle(saved[i].format, h)
		}
	}()
	for i, item := range saved {
		if item.bitmap != 0 {
			handles[i] = cloneClipboardBitmap(item.bitmap)
			if handles[i] == 0 {
				return errors.New("无法准备剪贴板图片恢复")
			}
			continue
		}
		h, _, _ := kernel32.NewProc("GlobalAlloc").Call(2, uintptr(len(item.data)))
		if h == 0 {
			return errors.New("无法准备剪贴板恢复")
		}
		handles[i] = h
		ptr, _, _ := kernel32.NewProc("GlobalLock").Call(h)
		if ptr == 0 {
			return errors.New("无法准备剪贴板恢复数据")
		}
		kernel32.NewProc("RtlMoveMemory").Call(ptr, uintptr(unsafe.Pointer(&item.data[0])), uintptr(len(item.data)))
		kernel32.NewProc("GlobalUnlock").Call(h)
	}
	if ok, _, _ := user32.NewProc("EmptyClipboard").Call(); ok == 0 {
		return errors.New("无法恢复剪贴板")
	}
	for i, item := range saved {
		if ok, _, _ := user32.NewProc("SetClipboardData").Call(uintptr(item.format), handles[i]); ok == 0 {
			return errors.New("剪贴板格式恢复失败")
		}
		handles[i] = 0 // ownership transferred to Windows
	}
	return nil
}

type copyKeyInput struct {
	Type        uint32
	Padding     uint32
	Key, Scan   uint16
	Flags, Time uint32
	Extra       uintptr
	Unused      [8]byte
}

func copySelection(ctx context.Context, event Event) (result Selection, failure error) {
	stage := "admission"
	defer func() {
		if os.Getenv("WINONE_SELECTION_DIAGNOSTICS") == "1" {
			fmt.Fprintf(os.Stderr, "selection copy: stage=%s status=%s\n", stage, result.Status)
		}
	}()
	select {
	case clipboardSlot <- struct{}{}:
		defer func() { <-clipboardSlot }()
	case <-ctx.Done():
		return Selection{}, ctx.Err()
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	current := func() bool { window, pid := Foreground(); return window == event.Window && pid == event.ProcessID }
	// Copy must never turn Ctrl+C into an interrupt or alter a held shortcut.
	if !current() || ctx.Err() != nil {
		return Selection{Status: "stale"}, nil
	}
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c, 0x43} {
		if modifier(key) {
			return Selection{Status: "unavailable"}, nil
		}
	}
	saved, before, err := snapshotClipboard()
	defer func() {
		for _, item := range saved {
			freeClipboardHandle(2, item.bitmap)
		}
	}()
	stage = "snapshot"
	if err != nil {
		return Selection{}, err
	}
	if !current() || ctx.Err() != nil || clipboardSequence() != before {
		return Selection{Status: "stale"}, nil
	}
	copyRevision := physicalCopyRevision.Load()
	keys := []copyKeyInput{{Type: 1, Key: 0x11, Extra: copyInputMarker}, {Type: 1, Key: 'C', Extra: copyInputMarker}, {Type: 1, Key: 'C', Flags: 2, Extra: copyInputMarker}, {Type: 1, Key: 0x11, Flags: 2, Extra: copyInputMarker}}
	count, _, _ := user32.NewProc("SendInput").Call(uintptr(len(keys)), uintptr(unsafe.Pointer(&keys[0])), unsafe.Sizeof(keys[0]))
	stage = "copy"
	if count != uintptr(len(keys)) {
		// Release only keys this transaction actually pressed; a partial
		// SendInput must not leave Ctrl or C held in the user's application.
		if count > 0 {
			releases := keys[2:]
			user32.NewProc("SendInput").Call(uintptr(len(releases)), uintptr(unsafe.Pointer(&releases[0])), unsafe.Sizeof(releases[0]))
		}
		return Selection{}, errors.New("目标程序未接受兼容复制按键")
	}
	// Poll only this bounded copy transaction, ending as soon as fresh data is
	// available. No fixed 100/600 ms wait on successful reads.
	deadline := time.Now().Add(180 * time.Millisecond)
	for clipboardSequence() == before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	after := clipboardSequence()
	if after == before {
		return Selection{Status: "empty"}, nil
	}
	class, _ := windows.UTF16PtrFromString("STATIC")
	restoreOwner, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if restoreOwner == 0 {
		return Selection{}, errors.New("无法创建剪贴板恢复窗口")
	}
	defer user32.NewProc("DestroyWindow").Call(restoreOwner)
	if err := openClipboardFor(restoreOwner, time.Now().Add(250*time.Millisecond)); err != nil {
		return Selection{}, err
	}
	defer user32.NewProc("CloseClipboard").Call()
	owner, _, _ := user32.NewProc("GetClipboardOwner").Call()
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(owner, uintptr(unsafe.Pointer(&pid)))
	// A different owner may be the user's concurrent copy. Leave it intact.
	if pid != event.ProcessID || physicalCopyRevision.Load() != copyRevision {
		if os.Getenv("WINONE_SELECTION_DIAGNOSTICS") == "1" {
			fmt.Fprintf(os.Stderr, "selection copy owner: expected=%d actual=%d\n", event.ProcessID, pid)
		}
		return Selection{Status: "stale"}, nil
	}
	defer func() {
		if err := restoreClipboardLocked(saved); err != nil {
			failure = err
			result = Selection{}
		}
	}()
	stage = "read"
	if !current() || ctx.Err() != nil {
		return Selection{Status: "stale"}, nil
	}
	data, err := clipboardBytes(13, 131074)
	if err != nil {
		return Selection{Status: "unavailable"}, nil
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		unit := uint16(data[i]) | uint16(data[i+1])<<8
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	text := string(utf16.Decode(units))
	if len(text) > 65536 {
		return Selection{Status: "too_large"}, nil
	}
	if strings.TrimSpace(text) == "" {
		return Selection{Status: "empty"}, nil
	}
	return Selection{Status: "selected", Text: text}, nil
}

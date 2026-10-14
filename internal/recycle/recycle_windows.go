//go:build windows

// Package recycle uses the Windows recycle operation and rejects its permanent
// deletion fallback. It has no resident process or UI of its own.
package recycle

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var ole32 = windows.NewLazySystemDLL("ole32.dll")
var shell32 = windows.NewLazySystemDLL("shell32.dll")
var coInitialize = ole32.NewProc("CoInitializeEx")
var coUninitialize = ole32.NewProc("CoUninitialize")
var coCreate = ole32.NewProc("CoCreateInstance")
var coFree = ole32.NewProc("CoTaskMemFree")
var shellItem = shell32.NewProc("SHCreateItemFromParsingName")
var classFileOperation = windows.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
var iidFileOperation = windows.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
var iidShellItem = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
var iidProgress = windows.GUID{Data1: 0x04b0f1a7, Data2: 0x9490, Data3: 0x44bc, Data4: [8]byte{0x96, 0xe1, 0x42, 0x96, 0xa3, 0x12, 0x52, 0xe2}}
var iidUnknown = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
var liveSinks sync.Map

type Receipt struct {
	Recycled bool   `json:"recycled"`
	Path     string `json:"path"`
}

func hresult(value uintptr) error {
	if int32(value) < 0 {
		return fmt.Errorf("Windows recycle operation failed (0x%08x)", uint32(value))
	}
	return nil
}

// Keep COM addresses as pointers while dereferencing them. uintptr is used
// only at the syscall boundary, so Go never reconstructs a pointer from an
// integer after a possible stack move.
type comObject struct{ vtable *uintptr }

//go:uintptrescapes
func invoke(object *comObject, index int, args ...uintptr) uintptr {
	method := *(*uintptr)(unsafe.Add(unsafe.Pointer(object.vtable), uintptr(index)*unsafe.Sizeof(uintptr(0))))
	result, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)...)
	runtime.KeepAlive(object)
	return result
}

// Directory accepts one existing, ordinary absolute directory. Ownership and
// package inventory checks belong to the caller; volume roots and links fail.
func Directory(ctx context.Context, path string) (Receipt, error) {
	return CheckedDirectory(ctx, path, nil)
}

// CheckedDirectory runs a final ownership check immediately before recycling.
func CheckedDirectory(ctx context.Context, path string, check func() error) (Receipt, error) {
	return checkedItem(ctx, path, true, false, check)
}

// CheckedFile shares the same no-permanent-deletion guarantee for known files.
func CheckedFile(ctx context.Context, path string, check func() error) (Receipt, error) {
	return checkedItem(ctx, path, false, false, check)
}

// CheckedLink recycles only an explicitly owned symlink/junction entry. It pins
// the observed reparse bytes and file identity and rejects any target traversal
// callback. Existing ordinary-file/directory entry points retain their rules.
func CheckedLink(ctx context.Context, path string, check func() error) (Receipt, error) {
	if check == nil {
		return Receipt{}, errors.New("link recycling requires an ownership check")
	}
	before, err := readRecycleLink(path)
	if err != nil {
		return Receipt{}, err
	}
	return checkedItem(ctx, path, false, true, func() error {
		after, err := readRecycleLink(path)
		if err != nil {
			return err
		}
		if before.volume != after.volume || before.id != after.id || !bytes.Equal(before.data, after.data) {
			return errors.New("link identity or target changed before recycling")
		}
		return check()
	})
}

type recycleLink struct {
	volume uint32
	id     uint64
	data   []byte
}

func readRecycleLink(path string) (recycleLink, error) {
	if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) {
		return recycleLink{}, errors.New("link must be an absolute non-root path")
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return recycleLink{}, err
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return recycleLink{}, err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		return recycleLink{}, err
	}
	var raw [16384]byte
	var size uint32
	if err = windows.DeviceIoControl(h, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &raw[0], uint32(len(raw)), &size, nil); err != nil {
		return recycleLink{}, err
	}
	if size < 8 {
		return recycleLink{}, errors.New("incomplete link data")
	}
	tag := binary.LittleEndian.Uint32(raw[:4])
	if tag != 0xa000000c && tag != 0xa0000003 {
		return recycleLink{}, errors.New("unsupported reparse type")
	}
	return recycleLink{info.VolumeSerialNumber, uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), append([]byte(nil), raw[:size]...)}, nil
}

func checkedItem(ctx context.Context, path string, directory, link bool, check func() error) (Receipt, error) {
	if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) {
		return Receipt{}, errors.New("recycle target must be an absolute non-root directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Receipt{}, err
	}
	// Go can classify junctions as ModeIrregular rather than ModeSymlink.
	// CheckedLink has already verified the native reparse tag and pins its raw
	// data for the final callback, so do not substitute Go's mode translation.
	if !link && (info.Mode()&os.ModeSymlink != 0 || directory && !info.IsDir() || !directory && !info.Mode().IsRegular()) {
		return Receipt{}, errors.New("recycle target has an unsupported type")
	}
	if err = ctx.Err(); err != nil {
		return Receipt{}, err
	}
	type result struct {
		receipt Receipt
		err     error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		receipt, err := directorySTA(ctx, filepath.Clean(path), check, link)
		done <- result{receipt, err}
	}()
	value := <-done
	return value.receipt, value.err
}

func directorySTA(ctx context.Context, path string, check func() error, strict bool) (Receipt, error) {
	value, _, _ := coInitialize.Call(0, 2) // COINIT_APARTMENTTHREADED
	if err := hresult(value); err != nil {
		return Receipt{}, err
	}
	defer coUninitialize.Call()
	var operation *comObject
	var sink *progressSink
	value, _, _ = coCreate.Call(uintptr(unsafe.Pointer(&classFileOperation)), 0, 23, uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	if err := hresult(value); err != nil {
		return Receipt{}, err
	}
	defer func() {
		invoke(operation, 2)
		if sink != nil {
			releaseSink(sink)
		}
		runtime.KeepAlive(sink)
	}()
	// No elevation or confirmation prompts. The progress sink aborts if Windows
	// cannot recycle, matching the safety check used by Electron's trash API.
	const flags = 0x0004 | 0x0010 | 0x0200 | 0x0400 | 0x00100000 | 0x00080000 | 0x20000000
	if err := hresult(invoke(operation, 5, flags)); err != nil {
		return Receipt{}, err
	}
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Receipt{}, err
	}
	var item *comObject
	value, _, _ = shellItem.Call(uintptr(unsafe.Pointer(wide)), 0, uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if err = hresult(value); err != nil {
		return Receipt{}, err
	}
	defer invoke(item, 2)
	sink = &progressSink{vtable: &sinkVtable, ctx: ctx, target: path, check: check, strict: strict}
	sink.refs.Store(1)
	liveSinks.Store(uintptr(unsafe.Pointer(sink)), sink)
	if err = hresult(invoke(operation, 18, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(sink)))); err != nil {
		return Receipt{}, err
	}
	err = hresult(invoke(operation, 21))
	var aborted int32
	statusErr := hresult(invoke(operation, 22, uintptr(unsafe.Pointer(&aborted))))
	runtime.KeepAlive(sink)
	if err != nil || statusErr != nil {
		return sink.receipt, errors.Join(err, statusErr, sink.err)
	}
	if aborted != 0 || !sink.receipt.Recycled {
		return sink.receipt, errors.Join(errors.New("recycle operation did not complete"), sink.err)
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return sink.receipt, errors.Join(errors.New("recycled source still exists or cannot be checked"), err)
	}
	return sink.receipt, sink.err
}

type progressSink struct {
	vtable  *[19]uintptr
	refs    atomic.Int32
	ctx     context.Context
	target  string
	receipt Receipt
	err     error
	check   func() error
	strict  bool
}

func releaseSink(sink *progressSink) uintptr {
	remaining := sink.refs.Add(-1)
	if remaining == 0 {
		liveSinks.Delete(uintptr(unsafe.Pointer(sink)))
	}
	return uintptr(remaining)
}

// TSF_DELETE_RECYCLE_IF_POSSIBLE is absent when Windows would permanently
// delete an item. Returning E_ABORT cancels this and subsequent operations.
func preDeleteDecision(ctx context.Context, flags uintptr) uintptr {
	if ctx.Err() != nil || flags&0x80 == 0 {
		return 0x80004004
	}
	return 0
}

func itemPath(item *comObject) (string, error) {
	var text *uint16
	if err := hresult(invoke(item, 5, 0x80058000, uintptr(unsafe.Pointer(&text)))); err != nil {
		return "", err
	} // SIGDN_FILESYSPATH
	if text == nil {
		return "", errors.New("recycled item has no filesystem path")
	}
	defer coFree.Call(uintptr(unsafe.Pointer(text)))
	return windows.UTF16PtrToString(text), nil
}

// Slots and signatures follow IFileOperationProgressSink in the Windows SDK.
// Allocate callback thunks once; NewCallback does not release individual thunks.
var sinkVtable = [19]uintptr{
	syscall.NewCallback(func(this *progressSink, riid *windows.GUID, out **progressSink) uintptr {
		if out == nil || riid == nil {
			return 0x80070057
		}
		*out = nil
		id := *riid
		if id != iidUnknown && id != iidProgress {
			return 0x80004002
		}
		sink := this
		sink.refs.Add(1)
		*out = this
		return 0
	}),
	syscall.NewCallback(func(this *progressSink) uintptr { return uintptr(this.refs.Add(1)) }),
	syscall.NewCallback(func(this *progressSink) uintptr { return releaseSink(this) }),
	syscall.NewCallback(func(this uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this, result uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this, flags, item, name uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, item, name, result, created uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, item, destination, name uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, item, destination, name, result, created uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, item, destination, name uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, item, destination, name, result, created uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this *progressSink, flags uintptr, item *comObject) uintptr {
		sink := this
		decision := preDeleteDecision(sink.ctx, flags)
		if decision != 0 {
			sink.err = errors.New("Windows cannot recycle this item or the operation was cancelled")
			return decision
		}
		if sink.check != nil {
			path, err := itemPath(item)
			if err == nil && sink.strict && !strings.EqualFold(path, sink.target) {
				err = errors.New("refusing to recycle a link target or child")
			}
			if err == nil && strings.EqualFold(path, sink.target) {
				err = sink.check()
			}
			if err != nil {
				sink.err = err
				return 0x80004004
			}
		}
		return decision
	}),
	syscall.NewCallback(func(this *progressSink, flags uintptr, item *comObject, result uintptr, created *comObject) uintptr {
		sink := this
		if err := hresult(result); err != nil {
			sink.err = err
			return 0
		}
		original, err := itemPath(item)
		if err != nil {
			sink.err = err
			return 0
		}
		if !strings.EqualFold(original, sink.target) {
			return 0
		}
		if created == nil {
			sink.err = errors.New("Windows did not return a recycle-bin item")
			return 0
		}
		path, err := itemPath(created)
		if err != nil {
			sink.err = err
			return 0
		}
		sink.receipt = Receipt{Recycled: true, Path: path}
		return 0
	}),
	syscall.NewCallback(func(this, flags, destination, name uintptr) uintptr { return 0x80004001 }),
	syscall.NewCallback(func(this, flags, destination, name, template, attributes, result, item uintptr) uintptr {
		return 0x80004001
	}),
	syscall.NewCallback(func(this *progressSink, total, done uintptr) uintptr {
		if this.ctx.Err() != nil {
			return 0x80004004
		}
		return 0
	}),
	syscall.NewCallback(func(this uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this uintptr) uintptr { return 0 }),
}

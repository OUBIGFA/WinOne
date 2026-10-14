//go:build windows

package recycle

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

var ErrNotFound = errors.New("recycled directory could not be confirmed")
var recycleFolder = windows.GUID{Data1: 0xb7534046, Data2: 0x3ecb, Data3: 0x4c18, Data4: [8]byte{0xbe, 0x4e, 0x64, 0xcd, 0x4c, 0xb7, 0xd6, 0xac}}
var enumItemsHandler = windows.GUID{Data1: 0x94f60519, Data2: 0x2850, Data3: 0x4924, Data4: [8]byte{0xaa, 0x5a, 0xd1, 0x5e, 0x84, 0x86, 0x80, 0x39}}
var iidEnumItems = windows.GUID{Data1: 0x70629033, Data2: 0xe363, Data3: 0x4a28, Data4: [8]byte{0xa5, 0x67, 0x0d, 0xb7, 0x80, 0x06, 0xe6, 0xd7}}
var iidShellItem2 = windows.GUID{Data1: 0x7e9fb0d3, Data2: 0x919f, Data3: 0x4307, Data4: [8]byte{0xab, 0x2e, 0x9b, 0x18, 0x60, 0x31, 0x0c, 0x93}}

type propertyKey struct {
	format windows.GUID
	id     uint32
}

// FindDirectory is read-only recovery for a recycle operation whose reply was
// lost. Match both the unique directory name and its original parent path;
// ambiguity is an error rather than a guess about unrelated recycle-bin items.
func FindDirectory(ctx context.Context, original string) (Receipt, error) {
	if !filepath.IsAbs(original) || filepath.Dir(filepath.Clean(original)) == filepath.Clean(original) {
		return Receipt{}, ErrNotFound
	}
	type result struct {
		receipt Receipt
		err     error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		receipt, err := findDirectorySTA(ctx, filepath.Clean(original))
		done <- result{receipt, err}
	}()
	value := <-done
	return value.receipt, value.err
}

func findDirectorySTA(ctx context.Context, original string) (Receipt, error) {
	value, _, _ := coInitialize.Call(0, 2)
	if err := hresult(value); err != nil {
		return Receipt{}, err
	}
	defer coUninitialize.Call()
	var folder *comObject
	value, _, _ = shell32.NewProc("SHGetKnownFolderItem").Call(uintptr(unsafe.Pointer(&recycleFolder)), 0, 0, uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&folder)))
	if err := hresult(value); err != nil {
		return Receipt{}, err
	}
	defer invoke(folder, 2)
	var enumerator *comObject
	if err := hresult(invoke(folder, 3, 0, uintptr(unsafe.Pointer(&enumItemsHandler)), uintptr(unsafe.Pointer(&iidEnumItems)), uintptr(unsafe.Pointer(&enumerator)))); err != nil {
		return Receipt{}, err
	}
	defer invoke(enumerator, 2)
	name, _ := windows.UTF16PtrFromString("System.Recycle.DeletedFrom")
	var deletedFrom propertyKey
	value, _, _ = windows.NewLazySystemDLL("propsys.dll").NewProc("PSGetPropertyKeyFromName").Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&deletedFrom)))
	if err := hresult(value); err != nil {
		return Receipt{}, err
	}
	var result Receipt
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var item *comObject
		var fetched uint32
		if err := hresult(invoke(enumerator, 3, 1, uintptr(unsafe.Pointer(&item)), uintptr(unsafe.Pointer(&fetched)))); err != nil {
			return result, err
		}
		if fetched == 0 {
			break
		}
		path, matched, err := matchRecycledDirectory(item, original, &deletedFrom)
		invoke(item, 2)
		if err != nil {
			return result, err
		}
		if !matched {
			continue
		}
		if result.Recycled {
			return Receipt{}, errors.New("multiple recycled directories match the operation")
		}
		result = Receipt{Recycled: true, Path: path}
	}
	if !result.Recycled {
		return result, ErrNotFound
	}
	return result, nil
}

func matchRecycledDirectory(item *comObject, original string, key *propertyKey) (string, bool, error) {
	var text *uint16
	if err := hresult(invoke(item, 5, 0, uintptr(unsafe.Pointer(&text)))); err != nil {
		return "", false, err
	}
	if text == nil {
		return "", false, errors.New("recycle-bin item has no name")
	}
	name := windows.UTF16PtrToString(text)
	coFree.Call(uintptr(unsafe.Pointer(text)))
	// Recycle-bin IShellItem implementations may return the full original path
	// for SIGDN_NORMALDISPLAY, while Shell automation returns only its name.
	if filepath.IsAbs(name) {
		if !strings.EqualFold(filepath.Clean(name), original) {
			return "", false, nil
		}
	} else if !strings.EqualFold(name, filepath.Base(original)) {
		return "", false, nil
	}
	var extended *comObject
	if err := hresult(invoke(item, 0, uintptr(unsafe.Pointer(&iidShellItem2)), uintptr(unsafe.Pointer(&extended)))); err != nil {
		return "", false, err
	}
	defer invoke(extended, 2)
	text = nil
	if err := hresult(invoke(extended, 17, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&text)))); err != nil {
		return "", false, err
	}
	if text == nil {
		return "", false, errors.New("recycled item has no original parent")
	}
	parent := windows.UTF16PtrToString(text)
	coFree.Call(uintptr(unsafe.Pointer(text)))
	if !strings.EqualFold(filepath.Clean(parent), filepath.Dir(original)) {
		return "", false, nil
	}
	path, err := itemPath(item)
	return path, err == nil, err
}

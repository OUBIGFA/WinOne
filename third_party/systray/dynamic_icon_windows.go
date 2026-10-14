//go:build windows

package systray

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/windows"
	"image/png"
	"sync"
)

var dynamicIconLock sync.Mutex
var retiredMenuBitmaps []windows.Handle
var deleteObject = g32.NewProc("DeleteObject")

func deleteMenuBitmap(handle windows.Handle) error {
	if handle == 0 {
		return nil
	}
	ok, _, err := deleteObject.Call(uintptr(handle))
	if ok == 0 {
		return fmt.Errorf("release menu bitmap: %v", err)
	}
	return nil
}
func retireMenuBitmap(handle windows.Handle) error {
	if err := deleteMenuBitmap(handle); err != nil {
		retiredMenuBitmaps = append(retiredMenuBitmaps, handle)
		return err
	}
	return nil
}
func flushRetiredMenuBitmaps() error {
	remaining := retiredMenuBitmaps[:0]
	var first error
	for _, handle := range retiredMenuBitmaps {
		if err := deleteMenuBitmap(handle); err != nil {
			remaining = append(remaining, handle)
			if first == nil {
				first = err
			}
		}
	}
	retiredMenuBitmaps = remaining
	return first
}

func pngMenuBitmap(data []byte) (windows.Handle, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if len(data) > 1<<20 {
		return 0, fmt.Errorf("menu PNG exceeds size limit")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 256 || config.Height > 256 {
		return 0, fmt.Errorf("invalid menu PNG")
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	return rgbaMenuBitmap(decoded)
}

// SetIconPNG updates an ephemeral Windows menu image without temporary files.
// Nil clears the icon. Failed updates leave the previous native icon in place.
func (item *MenuItem) SetIconPNG(data []byte) error {
	dynamicIconLock.Lock()
	defer dynamicIconLock.Unlock()
	if !wt.isReady() {
		return ErrTrayNotReadyYet
	}
	// Do not accumulate more native resources while an old bitmap is in use.
	if err := flushRetiredMenuBitmaps(); err != nil {
		return err
	}
	bitmap, err := pngMenuBitmap(data)
	if err != nil {
		return err
	}
	id := uint32(item.id)
	wt.muMenuItemIcons.Lock()
	old := wt.menuItemIcons[id]
	wt.menuItemIcons[id] = bitmap
	wt.muMenuItemIcons.Unlock()
	if err = wt.addOrUpdateMenuItem(id, item.parentId(), item.title, item.disabled, item.checked); err != nil {
		wt.muMenuItemIcons.Lock()
		wt.menuItemIcons[id] = old
		wt.muMenuItemIcons.Unlock()
		if cleanup := retireMenuBitmap(bitmap); cleanup != nil {
			return fmt.Errorf("%w; %v", err, cleanup)
		}
		return err
	}
	return retireMenuBitmap(old)
}

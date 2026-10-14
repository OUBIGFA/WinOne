//go:build windows

package systray

import "testing"

func TestMenuClickPreservesShiftAtDispatch(t *testing.T) {
	item := newMenuItem("Open", "", nil)
	defer func() { menuItemsLock.Lock(); delete(menuItems, item.id); menuItemsLock.Unlock() }()
	// Delivery may be consumed after the user releases Shift. The queued
	// event must retain the state captured by the native menu thread.
	systrayMenuItemSelectedWithShift(item.id, true)
	if shift := <-item.ClickedWithShiftCh; !shift {
		t.Fatal("Shift was lost")
	}
	systrayMenuItemSelectedWithShift(item.id, false)
	if shift := <-item.ClickedWithShiftCh; shift {
		t.Fatal("Previous Shift leaked")
	}
}

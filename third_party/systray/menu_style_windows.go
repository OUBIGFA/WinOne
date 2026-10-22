//go:build windows

package systray

import (
	"errors"
	"strings"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Native popup snapshots retain Win32 selection, focus and submenu handling.
// Only sizing and painting use WinOne's menu typography and neutral palette.
// https://learn.microsoft.com/windows/win32/menurc/using-menus#creating-owner-drawn-menu-items
var (
	menuGetInfo      = u32.NewProc("GetMenuItemInfoW")
	menuGetCount     = u32.NewProc("GetMenuItemCount")
	menuDrawText     = u32.NewProc("DrawTextW")
	menuFillRect     = u32.NewProc("FillRect")
	menuDrawState    = u32.NewProc("DrawStateW")
	menuCreateFont   = g32.NewProc("CreateFontW")
	menuCreateBrush  = g32.NewProc("CreateSolidBrush")
	menuDeleteObject = g32.NewProc("DeleteObject")
	menuSetTextColor = g32.NewProc("SetTextColor")
	menuSetBkMode    = g32.NewProc("SetBkMode")
	menuSaveDC       = g32.NewProc("SaveDC")
	menuRestoreDC    = g32.NewProc("RestoreDC")
	menuProcessHeap  = k32.NewProc("GetProcessHeap")
	menuHeapAlloc    = k32.NewProc("HeapAlloc")
	menuHeapFree     = k32.NewProc("HeapFree")
	menuAlphaBlend   = windows.NewLazySystemDLL("msimg32.dll").NewProc("AlphaBlend")
	menuBitmapInfo   = g32.NewProc("GetObjectW")
	menuRoundRect    = g32.NewProc("RoundRect")
	menuStockObject  = g32.NewProc("GetStockObject")
	menuFocusRect    = u32.NewProc("DrawFocusRect")
)

type menuRect struct{ Left, Top, Right, Bottom int32 }
type menuMeasure struct {
	Type, ControlID, ItemID, Width, Height uint32
	Data                                   uintptr
}
type menuDraw struct {
	Type, ControlID, ItemID, Action, State uint32
	Window, DC                             uintptr
	Rect                                   menuRect
	Data                                   uintptr
}
type menuBackground struct {
	Size, Mask, Style, MaxHeight uint32
	Brush                        uintptr
	HelpID                       uint32
	Data                         uintptr
}
type styledMenuEntry struct {
	title           string
	id, state, kind uint32
	bitmap, submenu windows.Handle
}

type nativeMenuStyle struct {
	frames                                     map[uintptr]bool
	frameError                                 error
	accessible                                 map[uintptr]int
	heap                                       uintptr
	root                                       windows.Handle
	font, backgroundBrush, selectedBrush       uintptr
	separatorBrush                             uintptr
	chevron                                    windows.Handle
	foreground, background, selected, disabled uint32
	dpi                                        uint32
	entries                                    []styledMenuEntry
	menus                                      map[windows.Handle][]int
	bitmaps                                    []windows.Handle
}

func newNativeMenuStyle(source windows.Handle, dark bool, dpi uint32) (*nativeMenuStyle, error) {
	if dpi < 96 || dpi > 768 {
		return nil, errors.New("unsupported menu DPI")
	}
	// Match the shared settings surface, hover and readable text tiers. Win32
	// still owns the popup frame, placement, keyboard input and system motion.
	s := &nativeMenuStyle{dpi: dpi, foreground: 0x262626, background: 0xffffff, selected: 0xe7e7e7, disabled: 0x636363, menus: make(map[windows.Handle][]int)}
	separator := uint32(0xe1e1e1)
	if dark {
		s.foreground = 0xe9e9e9
		s.background = 0x232323
		s.selected = 0x333333
		s.disabled = 0xaeaeae
		separator = 0x3f3f3f
	}
	s.accessible = make(map[uintptr]int)
	s.frames = make(map[uintptr]bool)
	s.heap, _, _ = menuProcessHeap.Call()
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	height := -s.px(14)
	s.font, _, _ = menuCreateFont.Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	s.backgroundBrush, _, _ = menuCreateBrush.Call(uintptr(s.background))
	s.selectedBrush, _, _ = menuCreateBrush.Call(uintptr(s.selected))
	s.separatorBrush, _, _ = menuCreateBrush.Call(uintptr(separator))
	if s.font == 0 || s.backgroundBrush == 0 || s.selectedBrush == 0 || s.separatorBrush == 0 {
		s.dispose()
		return nil, errors.New("could not allocate menu style")
	}
	chevron, err := loadMenuChevron(dark, s.px(16))
	if err != nil {
		s.dispose()
		return nil, err
	}
	s.chevron = chevron
	s.bitmaps = append(s.bitmaps, chevron)
	root, err := s.clone(source, 0)
	if err != nil {
		s.dispose()
		return nil, err
	}
	s.root = root
	return s, nil
}

func (s *nativeMenuStyle) px(value int32) int32 { return int32((int64(value)*int64(s.dpi) + 48) / 96) }

func readStyledMenuEntry(menu windows.Handle, position uint32) (styledMenuEntry, error) {
	info := menuItemInfo{Mask: 0x1c7}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, err := menuGetInfo.Call(uintptr(menu), uintptr(position), 1, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return styledMenuEntry{}, err
	}
	if info.Cch > 32768 {
		return styledMenuEntry{}, errors.New("menu text is too long")
	}
	buffer := make([]uint16, info.Cch+1)
	info.TypeData = &buffer[0]
	info.Cch = uint32(len(buffer))
	if ok, _, err := menuGetInfo.Call(uintptr(menu), uintptr(position), 1, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return styledMenuEntry{}, err
	}
	return styledMenuEntry{windows.UTF16ToString(buffer), info.ID, info.State, info.Type, info.BMPItem, info.SubMenu}, nil
}

// MSAAMENUINFO exposes owner-drawn labels to screen readers. Data and text live
// in the native heap, never in Go memory retained across a Windows callback.
func (s *nativeMenuStyle) clone(source windows.Handle, depth int) (result windows.Handle, failure error) {
	if depth > 16 {
		return 0, errors.New("menu nesting is too deep")
	}
	handle, _, err := pCreatePopupMenu.Call()
	if handle == 0 {
		return 0, err
	}
	menu := windows.Handle(handle)
	defer func() {
		if failure != nil {
			pDestroyMenu.Call(handle)
		}
	}()
	background := menuBackground{Mask: 2, Brush: s.backgroundBrush}
	background.Size = uint32(unsafe.Sizeof(background))
	if ok, _, err := pSetMenuInfo.Call(handle, uintptr(unsafe.Pointer(&background))); ok == 0 {
		return 0, err
	}
	count, _, err := menuGetCount.Call(uintptr(source))
	if int32(count) < 0 {
		return 0, err
	}
	for i := uint32(0); i < uint32(count); i++ {
		entry, err := readStyledMenuEntry(source, i)
		if err != nil {
			return 0, err
		}
		if entry.bitmap != 0 {
			if uintptr(entry.bitmap) <= 12 {
				return 0, errors.New("unsupported system menu bitmap")
			}
			copied, err := scaledMenuBitmap(entry.bitmap, s.px(20))
			if err != nil {
				return 0, err
			}
			s.bitmaps = append(s.bitmaps, copied)
			if templateMenuBitmaps[entry.bitmap] {
				if err := tintMenuBitmap(copied, s.foreground); err != nil {
					return 0, err
				}
			}
			entry.bitmap = copied
		}
		if entry.submenu != 0 {
			entry.submenu, err = s.clone(entry.submenu, depth+1)
			if err != nil {
				return 0, err
			}
		}
		s.entries = append(s.entries, entry)
		token := len(s.entries)
		s.menus[menu] = append(s.menus[menu], token)
		text, err := windows.UTF16PtrFromString(entry.title)
		if err != nil {
			if entry.submenu != 0 {
				pDestroyMenu.Call(uintptr(entry.submenu))
			}
			return 0, err
		}
		data, err := s.accessibleText(entry.title, token)
		if err != nil {
			if entry.submenu != 0 {
				pDestroyMenu.Call(uintptr(entry.submenu))
			}
			return 0, err
		}
		info := menuItemInfo{Mask: 0x167, Type: entry.kind | 0x100, State: entry.state, ID: entry.id, SubMenu: entry.submenu, ItemData: data, TypeData: text}
		info.Size = uint32(unsafe.Sizeof(info))
		if ok, _, err := pInsertMenuItem.Call(handle, uintptr(i), 1, uintptr(unsafe.Pointer(&info))); ok == 0 {
			if entry.submenu != 0 {
				pDestroyMenu.Call(uintptr(entry.submenu))
			}
			return 0, err
		}
	}
	return menu, nil
}

func (s *nativeMenuStyle) dispose() {
	if s == nil {
		return
	}
	if s.root != 0 {
		pDestroyMenu.Call(uintptr(s.root))
		s.root = 0
	}
	for _, bitmap := range s.bitmaps {
		menuDeleteObject.Call(uintptr(bitmap))
	}
	s.bitmaps = nil
	for address := range s.accessible {
		menuHeapFree.Call(s.heap, 0, address)
	}
	s.accessible = nil
	s.frames = nil
	for _, object := range []uintptr{s.font, s.backgroundBrush, s.selectedBrush, s.separatorBrush} {
		if object != 0 {
			menuDeleteObject.Call(object)
		}
	}
	s.font = 0
	s.backgroundBrush = 0
	s.selectedBrush = 0
	s.separatorBrush = 0
	s.chevron = 0
}

func (s *nativeMenuStyle) entry(token uintptr) (styledMenuEntry, bool) {
	index, ok := s.accessible[token]
	if !ok {
		return styledMenuEntry{}, false
	}
	return s.entries[index-1], true
}

type menuAccessibleInfo struct {
	Signature, Length uint32
	Text              uintptr
}

func (s *nativeMenuStyle) accessibleText(title string, index int) (uintptr, error) {
	text, err := windows.UTF16FromString(title)
	if err != nil {
		return 0, err
	}
	size := unsafe.Sizeof(menuAccessibleInfo{})
	address, _, err := menuHeapAlloc.Call(s.heap, 8, size+uintptr(len(text)*2))
	if address == 0 {
		return 0, err
	}
	info := (*menuAccessibleInfo)(unsafe.Pointer(address))
	info.Signature = 0xaa0df00d
	info.Length = uint32(len(text) - 1)
	info.Text = address + size
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(info.Text)), len(text)), text)
	s.accessible[address] = index
	return address, nil
}

func (s *nativeMenuStyle) measure(m *menuMeasure) bool {
	e, ok := s.entry(m.Data)
	if !ok || m.Type != 1 {
		return false
	}
	if e.kind&0x800 != 0 {
		// A separator follows its siblings' width; it never sizes the popup.
		m.Width = 0
		m.Height = uint32(s.px(13))
		return true
	}
	dc, _, _ := pGetDC.Call(0)
	if dc == 0 {
		return false
	}
	defer pReleaseDC.Call(0, dc)
	old, _, _ := pSelectObject.Call(dc, s.font)
	defer pSelectObject.Call(dc, old)
	text, _ := windows.UTF16PtrFromString(e.title)
	r := menuRect{}
	menuDrawText.Call(dc, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x420)
	// Measure the actual label plus the same gutters used by draw. Submenu
	// arrows reserve space only on rows that have one; no global minimum width.
	width := r.Right + s.px(42+12)
	if e.submenu != 0 {
		width += s.px(20)
	}
	m.Width = uint32(width)
	m.Height = uint32(s.px(40))
	return true
}

func (s *nativeMenuStyle) draw(d *menuDraw) bool {
	e, ok := s.entry(d.Data)
	if !ok || d.Type != 1 {
		return false
	}
	// The HDC belongs to the actual Win32 popup (including a submenu), not
	// the tray owner. Apply the shared system frame before painting its rows.
	s.preparePopupFrame(d.DC)
	saved, _, _ := menuSaveDC.Call(d.DC)
	defer menuRestoreDC.Call(d.DC, saved)
	menuFillRect.Call(d.DC, uintptr(unsafe.Pointer(&d.Rect)), s.backgroundBrush)
	if e.kind&0x800 != 0 {
		r := d.Rect
		r.Left += s.px(12)
		r.Right -= s.px(12)
		r.Top += (r.Bottom - r.Top) / 2
		r.Bottom = r.Top + 1 // One physical pixel, independent of menu DPI.
		menuFillRect.Call(d.DC, uintptr(unsafe.Pointer(&r)), s.separatorBrush)
		return true
	}
	if d.State&1 != 0 {
		// An inset hover row makes grouping legible without painting a second
		// popup border. Rounded geometry changes no item/hit-test coordinates.
		r := s.selectionRect(d.Rect)
		pen, _, _ := menuStockObject.Call(8) // NULL_PEN
		pSelectObject.Call(d.DC, pen)
		pSelectObject.Call(d.DC, s.selectedBrush)
		menuRoundRect.Call(d.DC, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(s.px(12)), uintptr(s.px(12)))
		if d.State&0x10 != 0 {
			menuFocusRect.Call(d.DC, uintptr(unsafe.Pointer(&r)))
		}
	}
	pSelectObject.Call(d.DC, s.font)
	menuSetBkMode.Call(d.DC, 1)
	color := s.foreground
	if d.State&6 != 0 {
		color = s.disabled
	}
	menuSetTextColor.Call(d.DC, uintptr(color))
	r := d.Rect
	r.Left += s.px(42)
	r.Right -= s.px(12)
	if e.submenu != 0 {
		r.Right -= s.px(20)
	}
	text, _ := windows.UTF16PtrFromString(e.title)
	menuDrawText.Call(d.DC, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x8024)
	if d.State&8 != 0 {
		r = d.Rect
		r.Left += s.px(12)
		r.Right = r.Left + s.px(20)
		mark, _ := windows.UTF16PtrFromString("✓")
		menuDrawText.Call(d.DC, uintptr(unsafe.Pointer(mark)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x25)
	} else if e.bitmap != 0 {
		x, y := d.Rect.Left+s.px(12), d.Rect.Top+(d.Rect.Bottom-d.Rect.Top-s.px(20))/2
		if d.State&6 != 0 {
			menuDrawState.Call(d.DC, 0, 0, uintptr(e.bitmap), 0, uintptr(x), uintptr(y), uintptr(s.px(20)), uintptr(s.px(20)), 0x24)
		} else if !s.drawBitmap(d.DC, e.bitmap, x, y) {
			return false
		}
	}
	if e.submenu != 0 {
		x, y := d.Rect.Right-s.px(28), d.Rect.Top+(d.Rect.Bottom-d.Rect.Top-s.px(16))/2
		if !s.drawBitmapSized(d.DC, s.chevron, x, y, s.px(16)) {
			return false
		}
	}
	return true
}

type nativeMenuBitmap struct {
	Type, Width, Height, Stride int32
	Planes, BitsPerPixel        uint16
	Bits                        uintptr
}

func (s *nativeMenuStyle) selectionRect(r menuRect) menuRect {
	r.Left += s.px(4)
	r.Right -= s.px(4)
	r.Top += s.px(2)
	r.Bottom -= s.px(2)
	return r
}

func (s *nativeMenuStyle) drawBitmap(target uintptr, bitmap windows.Handle, x, y int32) bool {
	return s.drawBitmapSized(target, bitmap, x, y, s.px(20))
}

func (s *nativeMenuStyle) drawBitmapSized(target uintptr, bitmap windows.Handle, x, y, size int32) bool {
	var info nativeMenuBitmap
	if ok, _, _ := menuBitmapInfo.Call(uintptr(bitmap), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); ok == 0 || info.Width <= 0 || info.Height <= 0 {
		return false
	}
	dc, _, _ := pCreateCompatibleDC.Call(target)
	if dc == 0 {
		return false
	}
	defer pDeleteDC.Call(dc)
	old, _, _ := pSelectObject.Call(dc, uintptr(bitmap))
	defer pSelectObject.Call(dc, old)
	// Menu bitmaps produced from icons are premultiplied BGRA. Preserve alpha
	// instead of copying transparent pixels as black onto a dark/light menu.
	ok, _, _ := menuAlphaBlend.Call(target, uintptr(x), uintptr(y), uintptr(size), uintptr(size), dc, 0, 0, uintptr(info.Width), uintptr(info.Height), 0x01ff0000)
	return ok != 0
}

// Duplicate mnemonics cycle selection; disabled rows and separators never
// execute. Arrow keys and Escape are still handled by TrackPopupMenu.
func (s *nativeMenuStyle) mnemonic(menu windows.Handle, character rune) uintptr {
	matches := []int{}
	selected := -1
	for position, token := range s.menus[menu] {
		e := s.entries[token-1]
		live := menuItemInfo{Mask: 1}
		live.Size = uint32(unsafe.Sizeof(live))
		if ok, _, _ := menuGetInfo.Call(uintptr(menu), uintptr(position), 1, uintptr(unsafe.Pointer(&live))); ok != 0 {
			e.state = live.State
		}
		if e.state&0x80 != 0 {
			selected = position
		}
		if e.kind&0x800 != 0 || e.state&3 != 0 {
			continue
		}
		runes := []rune(e.title)
		var key rune
		for i := 0; i < len(runes); i++ {
			if runes[i] == '&' && i+1 < len(runes) {
				if runes[i+1] == '&' {
					i++
					continue
				}
				key = runes[i+1]
				break
			}
		}
		if key == 0 {
			for _, r := range strings.TrimSpace(e.title) {
				key = r
				break
			}
		}
		if unicode.ToLower(key) == unicode.ToLower(character) {
			matches = append(matches, position)
		}
	}
	if len(matches) == 0 {
		return 0
	}
	index := matches[0]
	if len(matches) > 1 {
		for _, candidate := range matches {
			if candidate > selected {
				index = candidate
				break
			}
		}
		return uintptr(index) | 3<<16
	}
	return uintptr(index) | 2<<16
}

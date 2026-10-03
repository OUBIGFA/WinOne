//go:build windows

package systray

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func appendStyleFixture(t *testing.T, menu windows.Handle, id uint32, title string, state uint32, sub windows.Handle, separator bool) {
	t.Helper()
	text, err := windows.UTF16PtrFromString(title)
	if err != nil {
		t.Fatal(err)
	}
	info := menuItemInfo{Mask: 0x147, ID: id, TypeData: text, State: state, SubMenu: sub}
	if separator {
		info.Type = 0x800
	}
	info.Size = uint32(unsafe.Sizeof(info))
	count, _, _ := menuGetCount.Call(uintptr(menu))
	if ok, _, err := pInsertMenuItem.Call(uintptr(menu), count, 1, uintptr(unsafe.Pointer(&info))); ok == 0 {
		t.Fatal(err)
	}
}
func styleToken(s *nativeMenuStyle, index int) uintptr {
	for data, i := range s.accessible {
		if index == i {
			return data
		}
	}
	return 0
}

func TestNativeMenuSnapshotStyles(t *testing.T) {
	root, _, _ := pCreatePopupMenu.Call()
	child, _, _ := pCreatePopupMenu.Call()
	if root == 0 || child == 0 {
		t.Fatal("menu allocation failed")
	}
	defer pDestroyMenu.Call(root)
	appendStyleFixture(t, windows.Handle(child), 9, "恢复程序", 0, 0, false)
	appendStyleFixture(t, windows.Handle(root), 1, "&Open WinOne", 0, 0, false)
	appendStyleFixture(t, windows.Handle(root), 2, "&Other program", 0, windows.Handle(child), false)
	appendStyleFixture(t, windows.Handle(root), 3, "&Offline", 3, 0, false)
	appendStyleFixture(t, windows.Handle(root), 4, "", 0, 0, true)
	icon, err := create32BitHBitmap(0, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer menuDeleteObject.Call(icon)
	var iconInfo nativeMenuBitmap
	if ok, _, _ := menuBitmapInfo.Call(icon, unsafe.Sizeof(iconInfo), uintptr(unsafe.Pointer(&iconInfo))); ok == 0 || iconInfo.Bits == 0 {
		t.Fatal("icon buffer unavailable")
	}
	iconPixels := unsafe.Slice((*byte)(unsafe.Pointer(iconInfo.Bits)), 20*20*4)
	for i := 0; i < len(iconPixels); i += 4 {
		iconPixels[i+2] = 128
		iconPixels[i+3] = 128
	}
	bitmapItem := menuItemInfo{Mask: 0x80, BMPItem: windows.Handle(icon)}
	bitmapItem.Size = uint32(unsafe.Sizeof(bitmapItem))
	if ok, _, err := pSetMenuItemInfo.Call(root, 0, 1, uintptr(unsafe.Pointer(&bitmapItem))); ok == 0 {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, dpi := range []uint32{96, 144, 192} {
			s, err := newNativeMenuStyle(windows.Handle(root), dark, dpi)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer s.dispose()
				entry, err := readStyledMenuEntry(s.root, 0)
				if err != nil || entry.title != "&Open WinOne" || entry.kind&0x100 == 0 {
					t.Fatalf("lost native label/type: %+v %v", entry, err)
				}
				data := styleToken(s, s.menus[s.root][0])
				info := (*menuAccessibleInfo)(unsafe.Pointer(data))
				if info.Signature != 0xaa0df00d || windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(info.Text)), int(info.Length)+1)) != "&Open WinOne" {
					t.Fatal("missing accessible menu name")
				}
				m := menuMeasure{Type: 1, Data: data}
				if !s.measure(&m) || m.Width < uint32(s.px(200)) || m.Height != uint32(s.px(32)) {
					t.Fatalf("invalid measure %+v", m)
				}
				if s.mnemonic(s.root, 'o') != 3<<16 {
					t.Fatal("duplicate mnemonic did not select first enabled item")
				}
				highlight := menuItemInfo{Mask: 1, State: 0x80}
				highlight.Size = uint32(unsafe.Sizeof(highlight))
				pSetMenuItemInfo.Call(uintptr(s.root), 0, 1, uintptr(unsafe.Pointer(&highlight)))
				if s.mnemonic(s.root, 'o') != 1|3<<16 {
					t.Fatal("duplicate mnemonic did not cycle")
				}
				if _, ok := s.entry(1); ok {
					t.Fatal("foreign item data accepted")
				}
				renderStyleFixture(t, s, data, int(m.Width), int(m.Height), dark, dpi)
			}()
			if s.root != 0 || s.font != 0 || len(s.accessible) != 0 {
				t.Fatal("menu ownership was not released")
			}
		}
	}
	original, err := readStyledMenuEntry(windows.Handle(root), 0)
	if err != nil || original.kind&0x100 != 0 {
		t.Fatal("snapshot changed the original menu")
	}
}

func renderStyleFixture(t *testing.T, s *nativeMenuStyle, data uintptr, width, height int, dark bool, dpi uint32) {
	t.Helper()
	dc, _, _ := pCreateCompatibleDC.Call(0)
	if dc == 0 {
		t.Fatal("DC allocation failed")
	}
	defer pDeleteDC.Call(dc)
	bmi := bitmapInfo{BmiHeader: bitmapInfoHeader{BiWidth: int32(width), BiHeight: -int32(height * 2), BiPlanes: 1, BiBitCount: 32}}
	bmi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bmi.BmiHeader))
	var bits uintptr
	bitmap, _, err := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		t.Fatal(err)
	}
	defer menuDeleteObject.Call(bitmap)
	previous, _, _ := pSelectObject.Call(dc, bitmap)
	defer pSelectObject.Call(dc, previous)
	for row := 0; row < 2; row++ {
		d := menuDraw{Type: 1, Data: data, DC: dc, Rect: menuRect{0, int32(row * height), int32(width), int32((row + 1) * height)}}
		if row == 1 {
			d.State = 1
		}
		if !s.draw(&d) {
			t.Fatal("native drawing rejected item")
		}
	}
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(bits)), width*height*2*4)
	if uint32(pixels[0]) != s.background&255 || uint32(pixels[width*height*4]) != s.selected&255 {
		t.Fatal("menu did not use shared neutral background states")
	}
	center := (int(s.px(16))*width + int(s.px(18))) * 4
	expectedRed := 128 + int(s.background&255)*127/255
	if red := int(pixels[center+2]); red < expectedRed-1 || red > expectedRed+1 || pixels[center] >= pixels[center+2] {
		t.Fatal("icon alpha was not blended over the menu background")
	}
	if output := os.Getenv("WINONE_MENU_RENDER_OUTPUT"); output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, width, height*2))
		for i := 0; i < len(pixels); i += 4 {
			img.Pix[i] = pixels[i+2]
			img.Pix[i+1] = pixels[i+1]
			img.Pix[i+2] = pixels[i]
			img.Pix[i+3] = 255
		}
		name := "light"
		if dark {
			name = "dark"
		}
		name += "-" + strconv.Itoa(int(dpi))
		file, err := os.Create(filepath.Join(output, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, img)
		closeErr := file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}

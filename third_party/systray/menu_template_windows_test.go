//go:build windows

package systray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Exercise the same source API and popup snapshot as the Host, not a label or
// source-text assertion. Normal icons must retain their colour and ownership.
func TestTemplateMenuIconFollowsPopupThemeAndReleasesState(t *testing.T) {
	menu, _, err := pCreatePopupMenu.Call()
	if menu == 0 {
		t.Fatal(err)
	}
	wt.menus = map[uint32]windows.Handle{0: windows.Handle(menu)}
	wt.menuOf = make(map[uint32]windows.Handle)
	wt.visibleItems = make(map[uint32][]uint32)
	wt.menuItemIcons = make(map[uint32]windows.Handle)
	wt.initialized.Store(true)
	defer func() { wt.initialized.Store(false); pDestroyMenu.Call(menu) }()
	item := &MenuItem{id: 43, title: "Open WinOne"}
	defer item.Remove()
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := 16; y < 80; y++ {
		for x := 16; x < 80; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 192})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := item.SetTemplateIconPNG(encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	source := wt.menuItemIcons[item.id]
	for _, dark := range []bool{false, true} {
		for _, dpi := range []uint32{96, 120, 144, 192} {
			style, err := newNativeMenuStyle(windows.Handle(menu), dark, dpi)
			if err != nil {
				t.Fatal(err)
			}
			var info nativeMenuBitmap
			menuBitmapInfo.Call(uintptr(style.entries[0].bitmap), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info)))
			if info.Bits == 0 || info.Width != style.px(20) {
				t.Fatal("missing DPI-sized template")
			}
			pixels := unsafe.Slice((*byte)(unsafe.Pointer(info.Bits)), int(info.Stride*info.Height))
			i := int(info.Height/2*info.Stride + info.Width/2*4)
			want := byte((style.foreground & 255) * uint32(pixels[i+3]) / 255)
			if pixels[i] != want || pixels[i+1] != want || pixels[i+2] != want || pixels[i+3] == 0 {
				t.Fatalf("theme %v DPI %d lost template tint/alpha", dark, dpi)
			}
			style.dispose()
		}
	}
	if err := item.SetTemplateIconPNG([]byte("invalid")); err == nil || wt.menuItemIcons[item.id] != source {
		t.Fatal("failed replacement lost the old template")
	}
	if err := item.SetIconPNG(encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	if templateMenuBitmaps[source] {
		t.Fatal("replaced source retained template state")
	}
	style, err := newNativeMenuStyle(windows.Handle(menu), true, 96)
	if err != nil {
		t.Fatal(err)
	}
	var info nativeMenuBitmap
	menuBitmapInfo.Call(uintptr(style.entries[0].bitmap), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info)))
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(info.Bits)), int(info.Stride*info.Height))
	i := int(info.Height/2*info.Stride + info.Width/2*4)
	if pixels[i] != 0 || pixels[i+1] != 0 || pixels[i+2] == 0 {
		t.Fatal("normal program icon was tinted")
	}
	style.dispose()
	if err := item.SetTemplateIconPNG(encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	source = wt.menuItemIcons[item.id]
	if err := item.SetIconPNG(nil); err != nil {
		t.Fatal(err)
	}
	if templateMenuBitmaps[source] {
		t.Fatal("cleared template state leaked")
	}
	if err := item.SetTemplateIconPNG(encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	source = wt.menuItemIcons[item.id]
	item.Remove()
	if templateMenuBitmaps[source] {
		t.Fatal("removed template state leaked")
	}
}

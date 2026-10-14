//go:build windows

package systray

import (
	"bytes"
	"golang.org/x/sys/windows"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
	"unsafe"
)

func TestDynamicPNGDoesNotRetainGDIObjects(t *testing.T) {
	// USER/GDI initialize per-thread state lazily. Measure repeated updates on
	// one native thread so Go scheduling cannot count a new GUI thread as a leak.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			source.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	warm, err := pngMenuBitmap(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err = deleteMenuBitmap(warm); err != nil {
		t.Fatal(err)
	}
	resources := windows.NewLazySystemDLL("user32.dll").NewProc("GetGuiResources")
	before, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 0)
	beforeUser, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 1)
	for i := 0; i < 100; i++ {
		handle, err := pngMenuBitmap(encoded.Bytes())
		if err != nil || handle == 0 {
			t.Fatal(handle, err)
		}
		if err = deleteMenuBitmap(handle); err != nil {
			t.Fatal(err)
		}
	}
	after, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 0)
	afterUser, _, _ := resources.Call(uintptr(windows.CurrentProcess()), 1)
	if after > before+2 {
		t.Fatalf("GDI objects grew from %d to %d", before, after)
	}
	if afterUser > beforeUser+2 {
		t.Fatalf("USER objects grew from %d to %d", beforeUser, afterUser)
	}
	if _, err = pngMenuBitmap([]byte("broken")); err == nil {
		t.Fatal("invalid PNG accepted")
	}
}

func TestDynamicMenuIconReplacesClearsAndRemoves(t *testing.T) {
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
	item := &MenuItem{id: 42, title: "dynamic"}
	encoded := func(value color.RGBA) []byte {
		bitmap := image.NewRGBA(image.Rect(0, 0, 32, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				bitmap.SetRGBA(x, y, value)
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, bitmap); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	if err := item.SetIconPNG(encoded(color.RGBA{R: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	first := wt.menuItemIcons[42]
	if err := item.SetIconPNG(encoded(color.RGBA{B: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	second := wt.menuItemIcons[42]
	getObject := g32.NewProc("GetObjectW")
	var probe [64]byte
	if value, _, _ := getObject.Call(uintptr(first), uintptr(len(probe)), uintptr(unsafe.Pointer(&probe[0]))); value != 0 {
		t.Fatal("replaced bitmap still exists")
	}
	if err := item.SetIconPNG(nil); err != nil {
		t.Fatal(err)
	}
	var info menuItemInfo
	info.Size = uint32(unsafe.Sizeof(info))
	info.Mask = 0x80
	ok, _, err := u32.NewProc("GetMenuItemInfoW").Call(menu, 42, 0, uintptr(unsafe.Pointer(&info)))
	if ok == 0 || info.BMPItem != 0 {
		t.Fatal("menu icon did not clear", err)
	}
	if value, _, _ := getObject.Call(uintptr(second), uintptr(len(probe)), uintptr(unsafe.Pointer(&probe[0]))); value != 0 {
		t.Fatal("cleared bitmap still exists")
	}
	if err := item.SetIconPNG(encoded(color.RGBA{G: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	final := wt.menuItemIcons[42]
	item.Remove()
	if len(wt.menuItemIcons) != 0 {
		t.Fatal("removed bitmap remained in registry")
	}
	if value, _, _ := getObject.Call(uintptr(final), uintptr(len(probe)), uintptr(unsafe.Pointer(&probe[0]))); value != 0 {
		t.Fatal("removed bitmap still exists")
	}
}

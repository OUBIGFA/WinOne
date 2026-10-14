//go:build windows

package systray

import (
	"bytes"
	"golang.org/x/sys/windows"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"unsafe"
)

func diagonalMenuIcon(t *testing.T) windows.Handle {
	t.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x <= y; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	bitmap, err := pngMenuBitmap(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { menuDeleteObject.Call(uintptr(bitmap)) })
	return bitmap
}

func TestMenuPNGKeepsSourceResolution(t *testing.T) {
	bitmap := diagonalMenuIcon(t)
	var info nativeMenuBitmap
	if ok, _, _ := menuBitmapInfo.Call(uintptr(bitmap), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); ok == 0 {
		t.Fatal("missing bitmap")
	}
	if info.Width != 64 || info.Height != 64 {
		t.Fatalf("icon details discarded before menu scaling: %dx%d", info.Width, info.Height)
	}
}

func TestMenuScalesOnceForDPIWithSmoothTransparentEdges(t *testing.T) {
	bitmap := diagonalMenuIcon(t)
	root, _, _ := pCreatePopupMenu.Call()
	if root == 0 {
		t.Fatal("missing menu")
	}
	defer pDestroyMenu.Call(root)
	appendStyleFixture(t, windows.Handle(root), 1, "Collected program", 0, 0, false)
	item := menuItemInfo{Mask: 0x80, BMPItem: bitmap}
	item.Size = uint32(unsafe.Sizeof(item))
	if ok, _, err := pSetMenuItemInfo.Call(root, 0, 1, uintptr(unsafe.Pointer(&item))); ok == 0 {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		dpi  uint32
		size int32
	}{{96, 20}, {120, 25}, {144, 30}, {192, 40}} {
		style, err := newNativeMenuStyle(windows.Handle(root), false, sample.dpi)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer style.dispose()
			var info nativeMenuBitmap
			h := style.entries[0].bitmap
			if ok, _, _ := menuBitmapInfo.Call(uintptr(h), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); ok == 0 || info.Bits == 0 {
				t.Fatal("missing scaled pixels")
			}
			if info.Width != sample.size || info.Height != sample.size {
				t.Fatalf("DPI %d requires one %dpx resample, got %dx%d", sample.dpi, sample.size, info.Width, info.Height)
			}
			pixels := unsafe.Slice((*byte)(unsafe.Pointer(info.Bits)), int(info.Stride*info.Height))
			if pixels[(int(info.Height)-1)*int(info.Stride)+3] != 255 || pixels[(int(info.Width)-1)*4+3] != 0 {
				t.Fatal("menu icon orientation changed")
			}
			smooth := 0
			for i := 0; i < len(pixels); i += 4 {
				a := pixels[i+3]
				if a > 0 && a < 255 {
					smooth++
					if pixels[i+2] != a || pixels[i] != 0 || pixels[i+1] != 0 {
						t.Fatal("transparent edge acquired a dark fringe")
					}
				}
			}
			if smooth == 0 {
				t.Fatalf("DPI %d retained jagged binary edges", sample.dpi)
			}
			if output := os.Getenv("WINONE_MENU_ICON_OUTPUT"); output != "" {
				if err := os.MkdirAll(output, 0700); err != nil {
					t.Fatal(err)
				}
				preview := image.NewRGBA(image.Rect(0, 0, int(info.Width), int(info.Height)))
				for i := 0; i < len(pixels); i += 4 {
					preview.Pix[i], preview.Pix[i+1], preview.Pix[i+2], preview.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], pixels[i+3]
				}
				file, err := os.Create(filepath.Join(output, "icon-"+strconv.Itoa(int(sample.dpi))+".png"))
				if err != nil {
					t.Fatal(err)
				}
				encodeErr := png.Encode(file, preview)
				closeErr := file.Close()
				if encodeErr != nil || closeErr != nil {
					t.Fatal(encodeErr, closeErr)
				}
			}
		}()
	}
}

//go:build windows

package main

import (
	"bytes"
	"errors"
	"image/png"
	"strings"
	"testing"
)

type trayIconFixture struct {
	data    []byte
	failure error
}

func (f *trayIconFixture) SetTemplateIconPNG(data []byte) error {
	f.data = bytes.Clone(data)
	return f.failure
}

func TestTrayMenuIconsUseDistinctOfficialAlphaTemplates(t *testing.T) {
	open, quit := &trayIconFixture{}, &trayIconFixture{}
	if err := setTrayMenuIcons(open, quit); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(open.data, quit.data) {
		t.Fatal("open and exit have the same image")
	}
	for _, f := range []*trayIconFixture{open, quit} {
		img, err := png.Decode(bytes.NewReader(f.data))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 96 || img.Bounds().Dy() != 96 {
			t.Fatal("source resolution was discarded")
		}
		empty, ink := 0, 0
		for y := 0; y < 96; y++ {
			for x := 0; x < 96; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if r != g || g != b {
					t.Fatal("template source is not grayscale")
				}
				if a == 0 {
					empty++
				} else {
					ink++
				}
			}
		}
		if empty == 0 || ink == 0 {
			t.Fatal("template must contain both icon strokes and transparency")
		}
	}
}

func TestTrayMenuIconsReportBothFailuresWithoutSkippingExit(t *testing.T) {
	failure := errors.New("fixture bitmap failure")
	open, quit := &trayIconFixture{failure: failure}, &trayIconFixture{failure: failure}
	err := setTrayMenuIcons(open, quit)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "打开 WinOne") || !strings.Contains(err.Error(), "退出 WinOne") || len(quit.data) == 0 {
		t.Fatalf("icon failures were hidden or skipped: %v", err)
	}
}

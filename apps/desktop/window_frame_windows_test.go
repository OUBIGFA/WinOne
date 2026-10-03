//go:build windows

package main

import "testing"

func TestInitialWindowFitsWorkAreaAtDifferentDPI(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		dpi  uint32
	}{{1920, 1040, 96}, {3840, 2080, 144}, {1366, 728, 96}, {1920, 1040, 192}, {800, 600, 96}} {
		bounds, err := initialWindowBounds(tc.w, tc.h, tc.dpi)
		if err != nil {
			t.Fatal(err)
		}
		if bounds.Width > tc.w*96/int(tc.dpi) || bounds.Height > tc.h*96/int(tc.dpi) || bounds.MinWidth > bounds.Width || bounds.MinHeight > bounds.Height {
			t.Fatal(tc, bounds)
		}
	}
	large, err := initialWindowBounds(1920, 1040, 96)
	if err != nil || large.Width <= 1040 || large.Height <= 720 {
		t.Fatal("first window was not enlarged", large, err)
	}
	if _, err := initialWindowBounds(1920, 1040, 0); err == nil {
		t.Fatal("invalid DPI accepted")
	}
}

func TestWindowCommandsNeedValidActionAndWindow(t *testing.T) {
	app := &App{}
	for _, action := range []string{"unknown", "minimize", "toggle-maximize", "close"} {
		if err := app.WindowCommand(action); err == nil {
			t.Fatal("unavailable command reported success", action)
		}
	}
}

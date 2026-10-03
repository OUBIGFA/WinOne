//go:build windows

package main

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestScreenshotAndSettingsHaveIndependentWindowIdentities(t *testing.T) {
	settings := desktopRoleFor("pipe", false)
	capture := desktopRoleFor("pipe", true)
	if settings.InstanceID == capture.InstanceID || settings.WindowClass == capture.WindowClass || settings.WebviewDirectory == capture.WebviewDirectory {
		t.Fatal("window roles share ownership", settings, capture)
	}
	if desktopRoleFor("other-pipe", true).InstanceID == capture.InstanceID {
		t.Fatal("profiles share a screenshot instance")
	}
}

func TestCaptureAssetsServeDedicatedPageWithoutChangingSharedResources(t *testing.T) {
	source := fstest.MapFS{"index.html": {Data: []byte("settings")}, "capture.html": {Data: []byte("capture")}, "assets/shared.js": {Data: []byte("shared")}}
	for _, capture := range []bool{false, true} {
		page := desktopPageAssets(source, capture)
		data, err := fs.ReadFile(page, "index.html")
		if err != nil {
			t.Fatal(err)
		}
		want := "settings"
		if capture {
			want = "capture"
		}
		if string(data) != want {
			t.Fatal(string(data))
		}
		data, err = fs.ReadFile(page, "assets/shared.js")
		if err != nil || string(data) != "shared" {
			t.Fatal(string(data), err)
		}
	}
}

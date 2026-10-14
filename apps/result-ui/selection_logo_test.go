package resultui

import (
	"bytes"
	"image/png"
	"testing"
)

func TestToolbarKeepsTheCompleteOriginalLogo(t *testing.T) {
	original, err := png.DecodeConfig(bytes.NewReader(Logo()))
	if err != nil {
		t.Fatal(err)
	}
	width, height := toolbarLogo().Size()
	if width != original.Width || height != original.Height {
		t.Fatal("toolbar cropped or resized the source logo instead of fitting the complete image")
	}
}

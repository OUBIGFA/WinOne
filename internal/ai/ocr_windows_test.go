//go:build windows

package ai

import (
	"bytes"
	"context"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedLocalOCRRuntime(t *testing.T) {
	bundle := os.Getenv("WINONE_AI_RUNTIME_BUNDLE")
	if bundle == "" {
		t.Skip("set WINONE_AI_RUNTIME_BUNDLE to a complete test bundle")
	}
	bitmap := image.NewRGBA(image.Rect(0, 0, 600, 160))
	draw.Draw(bitmap, bitmap.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 32, DPI: 96, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	(&font.Drawer{Dst: bitmap, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(40, 85)}).DrawString("Hello world")
	var data bytes.Buffer
	if err = png.Encode(&data, bitmap); err != nil {
		t.Fatal(err)
	}
	text, err := (Translator{Executable: filepath.Join(bundle, "shared", "ai", "WinOne.AI.exe"), DataDirectory: t.TempDir()}).RecognizeLocal(context.Background(), data.Bytes())
	if err != nil || !strings.Contains(strings.ToLower(text), "hello world") {
		t.Fatalf("local OCR %q: %v", text, err)
	}
}

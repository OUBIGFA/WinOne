//go:build windows

package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestProgramInfoReadsNameAndIconWithoutStartingProgram(t *testing.T) {
	path := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	info, err := (&App{}).ProgramInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name == "" || info.Name == "explorer" {
		t.Fatalf("missing version description: %q", info.Name)
	}
	icon, err := png.Decode(bytes.NewReader(info.Icon))
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() != 32 || icon.Bounds().Dy() != 32 {
		t.Fatal(icon.Bounds())
	}
	visible := false
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			_, _, _, a := icon.At(x, y).RGBA()
			if a != 0 {
				visible = true
			}
		}
	}
	if !visible {
		t.Fatal("file icon was entirely transparent")
	}
}
func TestProgramInfoUsesScriptFilenameAndRejectsMissingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daily-backup.ps1")
	if err := os.WriteFile(path, []byte("# metadata fixture; never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := (&App{}).ProgramInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "daily-backup" {
		t.Fatal(info.Name)
	}
	if _, err = (&App{}).ProgramInfo(path + ".missing"); err == nil {
		t.Fatal("missing path accepted")
	}
}

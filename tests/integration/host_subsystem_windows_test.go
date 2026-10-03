//go:build windows

package integration

import (
	"debug/pe"
	"os"
	"testing"
)

func TestPackagedHostDoesNotAllocateConsole(t *testing.T) {
	path := os.Getenv("WINONE_HOST_EXE")
	if path == "" {
		t.Skip("set WINONE_HOST_EXE to inspect the packaged Host")
	}
	file, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	header, ok := file.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		t.Fatal("Host must be a Windows x64 executable")
	}
	if header.Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
		t.Fatalf("background Host uses console subsystem %d", header.Subsystem)
	}
}

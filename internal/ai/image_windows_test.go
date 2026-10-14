//go:build windows

package ai

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"testing"
)

func TestSharedImageSnapshotAndOwnership(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1cAAAAASUVORK5CYII=")
	peer := uint32(os.Getpid())
	name := fmt.Sprintf(`Local\WinOne.AI.%d.%s`, peer, strings.Repeat("a", 32))
	key, _ := windows.UTF16PtrFromString(name)
	mapping, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, 0, uint32(len(data)), key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(mapping)
	view, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_WRITE, 0, 0, uintptr(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.UnmapViewOfFile(view)
	if err = windows.WriteProcessMemory(windows.CurrentProcess(), view, &data[0], uintptr(len(data)), nil); err != nil {
		t.Fatal(err)
	}
	ref := ImageReference{Mapping: name, Bytes: len(data)}
	snapshot, err := ReadImage(ref, peer)
	if err != nil || !bytes.Equal(snapshot, data) {
		t.Fatalf("snapshot: %v", err)
	}
	zero := byte(0)
	if err = windows.WriteProcessMemory(windows.CurrentProcess(), view, &zero, 1, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot, data) {
		t.Fatal("AI request aliases mutable module memory")
	}
	for _, bad := range []ImageReference{{Mapping: name, Bytes: 0}, {Mapping: name, Bytes: 33 << 20}, {Mapping: "Local\\other", Bytes: len(data)}} {
		if _, err := ReadImage(bad, peer); err == nil {
			t.Fatal("invalid reference accepted")
		}
	}
	if _, err := ReadImage(ref, peer+1); err == nil {
		t.Fatal("another client's mapping accepted")
	}
	if _, err := ReadImage(ref, peer); err == nil {
		t.Fatal("non-PNG bytes accepted")
	}
}

//go:build windows

package selection

import (
	"testing"
	"unsafe"
)

func TestCopyInputMatchesWindowsABI(t *testing.T) {
	if unsafe.Sizeof(copyKeyInput{}) != 40 || unsafe.Offsetof(copyKeyInput{}.Extra) != 24 {
		t.Fatal("keyboard input layout does not match INPUT/KEYBDINPUT")
	}
}

func TestClipboardFormatsPreserveTextImagesHTMLAndFilesWithoutGuessingHandles(t *testing.T) {
	for _, format := range []uint32{1, 7, 8, 13, 15, 16, 17, 0xC001} {
		if !memoryClipboardFormat(format) {
			t.Fatalf("global-memory format refused: %d", format)
		}
	}
	for _, format := range []uint32{0, 2, 3, 9, 14, 0x80, 0x81, 0x200} {
		if memoryClipboardFormat(format) {
			t.Fatalf("non-memory format admitted: %d", format)
		}
	}
}

func TestClipboardOwnerMetadataIsNotReplayedAsContent(t *testing.T) {
	for _, name := range []string{"DataObject", "Ole Private Data"} {
		if !clipboardTransportFormat(name) {
			t.Fatalf("stale owner metadata would be restored: %s", name)
		}
	}
	for _, name := range []string{"HTML Format", "Rich Text Format", "PNG", "System.Drawing.Bitmap", "Custom Content"} {
		if clipboardTransportFormat(name) {
			t.Fatalf("user content would be discarded: %s", name)
		}
	}
}

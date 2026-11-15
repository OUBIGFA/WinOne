//go:build windows

package selection

import (
	"context"
	"errors"
	"testing"
	"time"
	"unsafe"
)

func TestCompatibilityCopyUsesCopyInsteadOfTerminalInterrupt(t *testing.T) {
	inputs := compatibilityCopyInputs()
	wantKeys := []uint16{0x11, 0x2d, 0x2d, 0x11}
	if len(inputs) != len(wantKeys) {
		t.Fatalf("incomplete copy chord: %+v", inputs)
	}
	for i, input := range inputs {
		wantFlags := []uint32{0, 1, 3, 2}[i]
		if input.Type != 1 || input.Key != wantKeys[i] || input.Flags != wantFlags || input.Extra != copyInputMarker {
			t.Fatalf("copy chord can interrupt or retrigger the watcher at input %d: %+v", i, input)
		}
	}
}

func TestCompatibilityCopyWaitsForSelectionModifiersToBeReleased(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reads := 0
	err := waitForCopyReady(ctx, func() (bool, bool) { reads++; return true, reads >= 3 })
	if err != nil || reads != 3 {
		t.Fatalf("selection chord was discarded before release: reads=%d error=%v", reads, err)
	}
}

func TestCompatibilityCopyWaitStopsWhenSourceChanges(t *testing.T) {
	reads := 0
	err := waitForCopyReady(context.Background(), func() (bool, bool) { reads++; return reads < 2, false })
	if err == nil || reads != 2 {
		t.Fatalf("source switch was not observed while waiting: reads=%d error=%v", reads, err)
	}
}

func TestCompatibilityCopyWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := waitForCopyReady(ctx, func() (bool, bool) { cancel(); return true, false })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read stayed eligible to copy: %v", err)
	}
}

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

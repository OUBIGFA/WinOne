package main

import (
	"os"
	"testing"
	"time"
)

func TestInterconnectEntryOpensDraftAndReleases(t *testing.T) {
	if os.Getenv("WINONE_INTERCONNECT_SYSTEM_TESTS") != "1" {
		t.Skip("explicit isolated native-window test only")
	}
	intents := make(chan bool, 2)
	entry := &interconnectEntry{open: func(paths []string, receive bool) error {
		if len(paths) != 0 {
			t.Error("button invented paths")
		}
		intents <- receive
		return nil
	}}
	defer entry.close()
	for _, receive := range []bool{false, true} {
		if err := entry.show(false); err != nil {
			t.Fatal(err)
		}
		window := entry.hwnd.Load()
		if window == 0 {
			t.Fatal("window did not open")
		}
		id := uintptr(100)
		if receive {
			id = 101
		}
		entryUser32.NewProc("SendMessageW").Call(window, 0x0111, id, 0)
		select {
		case actual := <-intents:
			if actual != receive {
				t.Fatal("wrong entry action")
			}
		case <-time.After(time.Second):
			t.Fatal("entry action did not arrive")
		}
		if err := entry.close(); err != nil {
			t.Fatal(err)
		}
		if result, _, _ := entryUser32.NewProc("IsWindow").Call(window); result != 0 {
			t.Fatal("native entry survived close")
		}
	}
}

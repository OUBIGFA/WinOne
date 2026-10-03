//go:build windows

package ipc

import (
	"fmt"
	"golang.org/x/sys/windows"
	"testing"
	"time"
)

func TestReadyWithdrawalReachesExistingObservers(t *testing.T) {
	name := fmt.Sprintf("WinOne-ready-release-%d", time.Now().UnixNano())
	release, err := MarkReady(name)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := windows.UTF16PtrFromString(name)
	observer, err := windows.OpenEvent(windows.SYNCHRONIZE, false, key)
	if err != nil {
		release()
		t.Fatal(err)
	}
	defer windows.CloseHandle(observer)
	release()
	release()
	if state, err := windows.WaitForSingleObject(observer, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("withdrawn marker stayed signalled: %d %v", state, err)
	}
	if WaitReady(name, 10*time.Millisecond, time.Millisecond) {
		t.Fatal("held observer handle made withdrawn marker ready")
	}
	again, err := MarkReady(name)
	if err != nil {
		t.Fatal(err)
	}
	defer again()
	if state, err := windows.WaitForSingleObject(observer, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("republished marker did not wake observer: %d %v", state, err)
	}
}

//go:build windows

package worker

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"testing"
	"time"
)

func TestLegacyPresenceGuard(t *testing.T) {
	name := fmt.Sprintf("WinOne-coexist-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	if err := ensureMutexAbsent(name); err != nil {
		t.Fatal(err)
	}
	key, _ := windows.UTF16PtrFromString(name)
	handle, err := windows.CreateMutex(nil, false, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMutexAbsent(name); err == nil {
		windows.CloseHandle(handle)
		t.Fatal("running instance ignored")
	}
	windows.CloseHandle(handle)
	if err := ensureMutexAbsent(name); err != nil {
		t.Fatal(err)
	}
}

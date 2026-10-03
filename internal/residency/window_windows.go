//go:build windows

package residency

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"golang.org/x/sys/windows"
	"unsafe"
)

func windowName(pipe string) string {
	return fmt.Sprintf(`Local\WinOne-Desktop-%x`, sha256.Sum256([]byte(pipe)))
}

// MarkWindow uses a kernel handle so a crashed UI cannot leave a stale presence file.
func MarkWindow(pipe string) (func(), error) {
	return markPresence(windowName(pipe))
}
func MarkAdministratorStart(pipe string) (func(), error) {
	return markPresence(windowName(pipe) + "-administrator")
}
func AdministratorStarting(pipe string) (bool, error) {
	return presenceExists(windowName(pipe) + "-administrator")
}
func markPresence(value string) (func(), error) {
	name, _ := windows.UTF16PtrFromString(value)
	handle, err := windows.CreateEvent(nil, 1, 1, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func(){windows.CloseHandle(handle)}) }, nil
}
func WindowOpen(pipe string) (bool, error) {
	return presenceExists(windowName(pipe))
}
func presenceExists(value string) (bool, error) {
	name, _ := windows.UTF16PtrFromString(value)
	h, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenEventW").Call(windows.SYNCHRONIZE, 0, uintptr(unsafe.Pointer(name)))
	if h != 0 {
		windows.CloseHandle(windows.Handle(h))
		return true, nil
	}
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	return false, err
}

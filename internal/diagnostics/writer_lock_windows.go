//go:build windows

package diagnostics

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

func withLogLock(directory string, work func() error) error {
	path, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	// Medium integrity permits the same user's elevated and ordinary workers
	// to synchronize, whichever process creates this short-lived mutex first.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")S:(ML;;NW;;;ME)")
	if err != nil {
		return err
	}
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Local\WinOne_Log_%x`, sha256.Sum256([]byte(strings.ToLower(path)))))
	handle, err := windows.CreateMutex(&security, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	defer windows.CloseHandle(handle)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		return fmt.Errorf("log writer lock unavailable: %d", status)
	}
	defer windows.ReleaseMutex(handle)
	return work()
}

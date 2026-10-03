//go:build windows

// Package maintenance coordinates runtime admission with installation work.
package maintenance

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"sync"
)

// Lease holds byte zero of maintenance.lock. Runtime leases are shared;
// maintenance is exclusive. The persistent file is never deleted or truncated.
type Lease struct {
	mu     sync.Mutex
	handle windows.Handle
}

func AcquireRuntime(directory string) (*Lease, error)     { return acquire(directory, false) }
func AcquireMaintenance(directory string) (*Lease, error) { return acquire(directory, true) }

func acquire(directory string, exclusive bool) (*Lease, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(directory, "maintenance.lock"))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if err = windows.LockFileEx(handle, flags, 0, 1, 0, &windows.Overlapped{}); err != nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("WinOne 正在运行或维护，请稍后重试：%w", err)
	}
	return &Lease{handle: handle}, nil
}

// Close releases the lease; Windows also releases it if the process crashes.
func (l *Lease) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(l.handle)
	if err == nil {
		l.handle = 0
	}
	return err
}

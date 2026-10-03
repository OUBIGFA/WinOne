//go:build windows

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// The stable sidecar survives atomic history replacement. Windows releases the
// byte-range lock on process exit, so a crashed importer cannot strand a lock.
func (h *StartupHistory) lock() (func() error, error) {
	if h == nil || h.path == "" {
		return nil, errors.New("startup history path is unavailable")
	}
	path, err := filepath.Abs(h.path)
	if err != nil {
		return nil, err
	}
	local, _ := historyLocks.LoadOrStore(strings.ToLower(filepath.Clean(path)), &sync.Mutex{})
	mu := local.(*sync.Mutex)
	mu.Lock()
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		mu.Unlock()
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	var overlapped windows.Overlapped
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || time.Now().After(deadline) {
			file.Close()
			mu.Unlock()
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return func() error {
		err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
		err = errors.Join(err, file.Close())
		mu.Unlock()
		return err
	}, nil
}

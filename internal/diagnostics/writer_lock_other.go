//go:build !windows

package diagnostics

import "sync"

// Non-Windows builds exercise serialization in portable unit tests only.
var portableLogLock sync.Mutex

func withLogLock(_ string, work func() error) error {
	portableLogLock.Lock()
	defer portableLogLock.Unlock()
	return work()
}

//go:build windows

package ai

import "errors"

// ProtectLocalData shares WinOne's existing Windows-user DPAPI boundary with
// local service adapters. It does not create a second credential store or use a
// portable encryption key; callers own the path and never expose plaintext.
func ProtectLocalData(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, errors.New("invalid protected data size")
	}
	return protect(data, false)
}
func UnprotectLocalData(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, errors.New("invalid protected data size")
	}
	return protect(data, true)
}

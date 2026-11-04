//go:build !windows

package trystero

import "syscall"

func readAdapters() (physicalAdapters, error) { return physicalAdapters{}, nil }

func unicastInterface(int, bool) func(string, string, syscall.RawConn) error { return nil }

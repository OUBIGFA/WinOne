//go:build windows

package main

import "winone/internal/distribution"

func backupRegistration(directory, reference, backup, key string) error {
	return distribution.BackupRegistration(directory, reference, backup, key)
}

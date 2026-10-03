//go:build !windows

package main

import "errors"

func backupRegistration(directory, reference, backup, key string) error {
	return errors.New("registration backup requires Windows")
}

//go:build windows

package main

import "winone/internal/adminstartup"

// Both desktop entrypoints use the same authorized-task handoff.
func runAuthorizedStartHelper(args []string, executable string) (bool, error) {
	return adminstartup.RunAuthorizedStartHelper(args, executable)
}

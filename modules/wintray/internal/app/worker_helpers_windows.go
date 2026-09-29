//go:build windows

package app

// RunWorkerHelper preserves the upstream task helper protocol without starting its settings UI.
// A negative return value means this is not a helper invocation.
func RunWorkerHelper(args []string) int { return runAppTaskHelper(args) }

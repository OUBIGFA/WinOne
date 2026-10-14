//go:build !windows

package processlifetime

import "os"

func ExitAfterCleanup(code int) { os.Exit(code) }

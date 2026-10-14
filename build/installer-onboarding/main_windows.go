//go:build windows

// This fixed-purpose, standard-user helper prepares first-use storage only.
// It never claims a hint, changes existing ACLs, or accepts a runtime path.
package main

import (
	"fmt"
	"os"
	"winone/internal/onboarding"
	"winone/internal/processlifetime"
)

func main() {
	code := 0
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "首次提示准备程序不接受参数")
		code = 1
	} else {
		directory, err := onboarding.MachineDirectory()
		if err == nil {
			err = onboarding.PrepareDirectory(directory)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	processlifetime.ExitAfterCleanup(code)
}

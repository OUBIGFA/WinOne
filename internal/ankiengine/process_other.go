//go:build !windows

package ankiengine

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

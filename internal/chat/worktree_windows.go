//go:build windows

package chat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// CreateDetachedWorktree keeps the selected repository authoritative even if
// WinOne inherited GIT_DIR/INDEX variables from the terminal that launched it.
func CreateDetachedWorktree(ctx context.Context, directory, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", directory, "worktree", "add", "--detach", target, "HEAD")
	command.Env = gitReadEnvironment(os.Environ())
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	command.WaitDelay = time.Second
	var output gitOutput
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("无法创建工作树：%w；%s", err, output.String())
	}
	return nil
}

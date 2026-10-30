//go:build windows

package chat

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// WorktreeState answers Cindy's detect-cwd question for automation forms: is
// Git available, is the folder a repository, and is it already a linked
// worktree (where an isolated automation worktree cannot be created).
type WorktreeState struct {
	GitInstalled     bool   `json:"gitInstalled"`
	IsGitRepo        bool   `json:"isGitRepo"`
	IsInsideWorktree bool   `json:"isInsideWorktree"`
	CurrentBranch    string `json:"currentBranch,omitempty"`
	RepoRoot         string `json:"repoRoot,omitempty"`
}

func ReadWorktreeState(ctx context.Context, directory string) (WorktreeState, error) {
	directory, err := existingDirectory(directory)
	if err != nil {
		return WorktreeState{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return WorktreeState{}, nil
	}
	state := WorktreeState{GitInstalled: true}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	run := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, git, append([]string{"-C", directory}, args...)...)
		command.Env = gitReadEnvironment(os.Environ())
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		command.WaitDelay = time.Second
		var output, diagnostic gitOutput
		command.Stdout, command.Stderr = &output, &diagnostic
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			var failed *exec.ExitError
			if errors.As(err, &failed) {
				return "", nil
			}
			return "", err
		}
		return strings.TrimSpace(output.String()), nil
	}
	values, err := run("rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return state, err
	}
	lines := strings.Split(values, "\n")
	if len(lines) < 3 {
		return state, nil
	}
	state.IsGitRepo = true
	state.RepoRoot = filepath.Clean(strings.TrimSpace(lines[0]))
	gitDir, commonDir := filepath.Clean(strings.TrimSpace(lines[1])), filepath.Clean(strings.TrimSpace(lines[2]))
	state.IsInsideWorktree = !strings.EqualFold(gitDir, commonDir)
	if branch, err := run("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		state.CurrentBranch = branch
	}
	return state, nil
}

//go:build windows

package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// GitState describes HEAD without checking out files or changing the index.
// An empty repository has a real symbolic branch; detached HEAD has a commit.
type GitState struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Detached   bool   `json:"detached,omitempty"`
	Unborn     bool   `json:"unborn,omitempty"`
}

type gitOutput struct{ bytes.Buffer }

func (b *gitOutput) Write(value []byte) (int, error) {
	if b.Len()+len(value) > 64<<10 {
		return 0, errors.New("Git 输出超过限制")
	}
	return b.Buffer.Write(value)
}

func gitReadEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		name = strings.ToUpper(name)
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_PREFIX", "GIT_CEILING_DIRECTORIES", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_OPTIONAL_LOCKS", "LC_ALL", "LANG":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "LANG=C")
}

func ReadGitState(ctx context.Context, directory string) (GitState, error) {
	directory, err := existingDirectory(directory)
	if err != nil {
		return GitState{}, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return GitState{}, errors.New("未找到 Git，无法读取项目分支")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	run := func(args ...string) (string, string, int, error) {
		command := exec.CommandContext(ctx, git, append([]string{"-C", directory}, args...)...)
		command.Env = gitReadEnvironment(os.Environ())
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		command.WaitDelay = time.Second
		var output, diagnostic gitOutput
		command.Stdout = &output
		command.Stderr = &diagnostic
		err := command.Run()
		if ctx.Err() != nil {
			return "", "", -1, ctx.Err()
		}
		code := 0
		if err != nil {
			var failed *exec.ExitError
			if errors.As(err, &failed) {
				code = failed.ExitCode()
			} else {
				return "", "", -1, err
			}
		}
		return strings.TrimSpace(output.String()), strings.TrimSpace(diagnostic.String()), code, err
	}
	_, diagnostic, code, err := run("rev-parse", "--git-dir")
	if err != nil {
		if code == 128 && strings.HasPrefix(diagnostic, "fatal: not a git repository (or any of the parent directories)") {
			return GitState{}, nil
		}
		return GitState{}, gitReadError(err, diagnostic)
	}
	branch, diagnostic, code, err := run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		if branch == "" || !utf8.ValidString(branch) {
			return GitState{}, errors.New("Git 返回了无效的分支名称")
		}
		_, detail, headCode, headErr := run("rev-parse", "--verify", "--quiet", "HEAD")
		if headErr != nil && headCode != 1 {
			return GitState{}, gitReadError(headErr, detail)
		}
		return GitState{Repository: true, Branch: branch, Unborn: headCode == 1}, nil
	}
	if code != 1 {
		return GitState{}, gitReadError(err, diagnostic)
	}
	commit, diagnostic, _, err := run("rev-parse", "--verify", "--short=8", "HEAD")
	if err != nil {
		return GitState{}, gitReadError(err, diagnostic)
	}
	if commit == "" || !utf8.ValidString(commit) {
		return GitState{}, errors.New("Git 返回了无效的 HEAD")
	}
	return GitState{Repository: true, Detached: true, Commit: commit}, nil
}
func gitReadError(err error, diagnostic string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if diagnostic == "" {
		return fmt.Errorf("读取 Git 分支失败：%w", err)
	}
	if len(diagnostic) > 1200 {
		diagnostic = diagnostic[:1200]
	}
	return fmt.Errorf("读取 Git 分支失败：%s", strings.ToValidUTF8(diagnostic, "�"))
}

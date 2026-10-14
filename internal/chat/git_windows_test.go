//go:build windows

package chat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitFixture(t *testing.T, directory string, args ...string) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	command := exec.Command(git, append([]string{"-C", directory, "-c", "core.hooksPath=", "-c", "user.name=WinOne fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
	command.Env = append(gitReadEnvironment(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s %v", args, output, err)
	}
	return string(output)
}

func TestProjectGitReadsActualUnbornBranchAndNonRepository(t *testing.T) {
	directory := t.TempDir()
	gitFixture(t, directory, "init", "--initial-branch=feature/actual")
	state, err := ReadGitState(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Repository || state.Branch != "feature/actual" || !state.Unborn || state.Detached {
		t.Fatal("Git HEAD was replaced with a default", state)
	}
	child := filepath.Join(directory, "nested")
	if err = os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	inherited, err := ReadGitState(context.Background(), child)
	if err != nil || inherited.Branch != "feature/actual" {
		t.Fatal("project subdirectory lost its real parent repository", inherited, err)
	}
	t.Run("non-repository", func(t *testing.T) {
		// Build tests intentionally keep TEMP below WinOne's repository. Such a
		// temp folder is a genuine Git subdirectory, so use a read-only volume
		// root for this case; no file is created or changed outside the fixture.
		root := filepath.VolumeName(directory) + string(os.PathSeparator)
		none, err := ReadGitState(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if none.Repository {
			t.Skip("the volume root itself is a Git repository; no isolated non-repository exists on this volume")
		}
		if none.Branch != "" {
			t.Fatal("non-repository acquired a branch", none)
		}
	})
}

func TestAutomationCreatesDetachedWorktreeWithoutChangingSource(t *testing.T) {
	directory := t.TempDir()
	gitFixture(t, directory, "init", "--initial-branch=topic/source")
	gitFixture(t, directory, "commit", "--allow-empty", "-m", "fixture")
	commit := strings.TrimSpace(gitFixture(t, directory, "rev-parse", "--short=8", "HEAD"))
	original, err := ReadGitState(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "worktree")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "wrong"))
	if err = CreateDetachedWorktree(context.Background(), directory, target); err != nil {
		t.Fatal(err)
	}
	created, err := ReadGitState(context.Background(), target)
	if err != nil || !created.Detached || created.Commit != commit {
		t.Fatalf("%+v %v", created, err)
	}
	after, err := ReadGitState(context.Background(), directory)
	if err != nil || after.Branch != original.Branch || strings.TrimSpace(gitFixture(t, directory, "rev-parse", "--short=8", "HEAD")) != commit {
		t.Fatalf("source changed: %+v %v", after, err)
	}
}

func TestProjectGitHandlesDetachedWorktreeAndIgnoresInheritedBinding(t *testing.T) {
	directory := t.TempDir()
	gitFixture(t, directory, "init", "--initial-branch=topic/one")
	gitFixture(t, directory, "commit", "--allow-empty", "-m", "fixture")
	worktree := filepath.Join(t.TempDir(), "linked tree")
	gitFixture(t, directory, "worktree", "add", "--detach", worktree, "HEAD")
	state, err := ReadGitState(context.Background(), worktree)
	if err != nil || !state.Repository || !state.Detached || state.Branch != "" || len(state.Commit) < 8 {
		t.Fatal("detached worktree is not represented accurately", state, err)
	}
	other := t.TempDir()
	gitFixture(t, other, "init", "--initial-branch=topic/two")
	t.Setenv("GIT_DIR", filepath.Join(directory, ".git"))
	t.Setenv("GIT_WORK_TREE", directory)
	state, err = ReadGitState(context.Background(), other)
	if err != nil || state.Branch != "topic/two" {
		t.Fatal("inherited Git environment overrode selected project", state, err)
	}
}

func TestProjectGitDistinguishesMissingToolFromNoRepository(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := ReadGitState(context.Background(), t.TempDir()); err == nil {
		t.Fatal("missing Git reported no branch without an error")
	}
}

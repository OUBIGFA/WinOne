//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"winone/internal/chat"
)

// The automation page may open only automation or project folders and the
// project's .cindy/automations/schedules.json; other paths are refused before
// any shell call.
func TestAutomationOpenRefusesPathsOutsideAutomations(t *testing.T) {
	profile := t.TempDir()
	store, err := chat.Open(filepath.Join(profile, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(profile, "project")
	if err := os.MkdirAll(filepath.Join(project, ".cindy", "automations"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// No bundled scheduler: the allow-list comes from Chat projects only.
	automations := newChatAutomations(&chatControl{ctx: ctx, directory: profile, desktop: filepath.Join(profile, "missing", "WinOne.exe"), store: store})
	open := func(path string) error {
		raw, _ := json.Marshal(map[string]string{"path": path})
		_, err := automations.openPath(ctx, raw)
		return err
	}
	for _, path := range []string{"relative", filepath.Join(profile, "elsewhere"), filepath.Join(project, "notes.txt"), filepath.Join(project, "schedules.json"), filepath.Join(project, ".cindy", "automations", "schedules.json")} {
		if err := open(path); err == nil {
			t.Fatalf("opened %s", path)
		}
	}
}

func TestAutomationWorktreeDetection(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	repository := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(git, append([]string{"-C", repository}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, output)
		}
	}
	run("init", "-q", "-b", "main")
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-q", "--allow-empty", "-m", "init")
	state, err := chat.ReadWorktreeState(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GitInstalled || !state.IsGitRepo || state.IsInsideWorktree || state.CurrentBranch != "main" {
		t.Fatalf("repository state %+v", state)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	run("worktree", "add", "-q", "--detach", linked)
	state, err = chat.ReadWorktreeState(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsGitRepo || !state.IsInsideWorktree {
		t.Fatalf("linked worktree state %+v", state)
	}
	plainDirectory := t.TempDir()
	// Tests run under the repository's _temp tree. A non-repository Git marker
	// stops discovery at this fixture instead of finding WinOne's parent .git.
	if err := os.WriteFile(filepath.Join(plainDirectory, ".git"), []byte("gitdir: missing-fixture-repository\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plain, err := chat.ReadWorktreeState(context.Background(), plainDirectory)
	if err != nil || plain.IsGitRepo {
		t.Fatalf("plain folder %+v %v", plain, err)
	}
}

//go:build windows

package chat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewPushRequiresConfirmationAndRejectsStaleLease(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	run := func(dir string, args ...string) string {
		t.Helper()
		out, err := reviewGit(ctx, dir, "", args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	remote := filepath.Join(root, "remote.git")
	local := filepath.Join(root, "local")
	other := filepath.Join(root, "other")
	for _, dir := range []string{remote, local} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	run(remote, "init", "--bare", "--initial-branch=main")
	// A descriptive repository-local TEMP can make incoming object paths exceed
	// MAX_PATH. Keep the Windows long-path setting inside these fixtures.
	run(remote, "config", "core.longpaths", "true")
	run(local, "init", "--initial-branch=main")
	identity := func(dir string) {
		run(dir, "config", "core.longpaths", "true")
		run(dir, "config", "user.name", "Review test")
		run(dir, "config", "user.email", "review@example.test")
	}
	commit := func(dir, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		run(dir, "add", "--", "note.txt")
		run(dir, "commit", "-m", text)
	}
	identity(local)
	commit(local, "base\n")
	run(local, "remote", "add", "origin", remote)
	pushed, err := PushReview(ctx, local, nil)
	if err != nil || pushed.Kind != "pushed" {
		t.Fatalf("first push: %+v %v", pushed, err)
	}
	run(root, "-c", "core.longpaths=true", "clone", "--config", "core.longpaths=true", "--branch", "main", "--", remote, other)
	identity(other)
	commit(other, "remote\n")
	run(other, "push")
	remoteBefore, _ := reviewRef(ctx, remote, "HEAD")
	commit(local, "local\n")
	rejected, err := PushReview(ctx, local, nil)
	if err != nil || rejected.Kind != "needs-force" || rejected.Ahead != 1 || rejected.Behind != 1 {
		t.Fatalf("rejection: %+v %v", rejected, err)
	}
	if after, _ := reviewRef(ctx, remote, "HEAD"); after != remoteBefore {
		t.Fatal("ordinary push changed a diverged remote")
	}
	lease := &ReviewPushLease{RemoteRef: rejected.RemoteRef, ExpectedOID: rejected.RemoteOID}
	commit(other, "remote changed while confirming\n")
	run(other, "push")
	latest, _ := reviewRef(ctx, remote, "HEAD")
	if _, err = PushReview(ctx, local, lease); err == nil {
		t.Fatal("stale confirmation overwrote a new remote commit")
	}
	if after, _ := reviewRef(ctx, remote, "HEAD"); after != latest {
		t.Fatal("stale lease changed remote")
	}
	rejected, err = PushReview(ctx, local, nil)
	if err != nil || rejected.Kind != "needs-force" {
		t.Fatalf("refresh: %+v %v", rejected, err)
	}
	pushed, err = PushReview(ctx, local, &ReviewPushLease{RemoteRef: rejected.RemoteRef, ExpectedOID: rejected.RemoteOID})
	if err != nil || pushed.Kind != "pushed" {
		t.Fatalf("confirmed push: %+v %v", pushed, err)
	}
	want, _ := reviewRef(ctx, local, "HEAD")
	if after, _ := reviewRef(ctx, remote, "HEAD"); after != want {
		t.Fatal("confirmed push did not publish the selected branch")
	}
}

package workspace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

const reviewPatch = "diff --git a/old.go b/new.go\n--- a/old.go\n+++ b/new.go\n@@ -1,5 +1,5 @@\n a\n-old1\n+new1\n middle\n-old2\n+new2\n z\n@@ -20,1 +20,1 @@\n-old3\n+new3\n"

type reviewFixture struct {
	mu        sync.Mutex
	mutations []map[string]any
	pushes    []map[string]any
}

func reviewSetup(t *testing.T) (*reviewFixture, *testhost.Harness, *ReviewPane, *ui.Tester) {
	t.Helper()
	f := &reviewFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, reviewPatch) }))
	t.Cleanup(server.Close)
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]any
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if input["sessionId"] != "owned" {
			return nil, fmt.Errorf("review escaped its conversation")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch method {
		case "chat.review.snapshot":
			return ReviewSnapshot{Root: `C:\work`, Branch: "main", Scope: "working", Revision: "snapshot-one", Files: []ReviewFile{{Path: "new.go", OldPath: "old.go", Status: "R", Additions: 3, Deletions: 3}}}, nil
		case "chat.review.metadata":
			return ReviewMetadata{Repository: true, Root: `C:\work`, Branch: "main", HeadOID: strings.Repeat("b", 40)}, nil
		case "chat.review.commits":
			return []ReviewCommit{}, nil
		case "chat.review.diff.preview":
			if input["path"] != "new.go" || input["revision"] != "snapshot-one" {
				return nil, fmt.Errorf("wrong diff identity")
			}
			return map[string]string{"url": server.URL}, nil
		case "chat.review.mutate":
			f.mutations = append(f.mutations, input["mutation"].(map[string]any))
			return nil, nil
		case "chat.review.push":
			f.pushes = append(f.pushes, input)
			if input["confirmForce"] == nil {
				return ReviewPushResult{Kind: "needs-force", Remote: "origin", RemoteRef: "refs/heads/main", RemoteOID: strings.Repeat("a", 40), Ahead: 2, Behind: 1}, nil
			}
			return ReviewPushResult{Kind: "pushed", Remote: "origin", RemoteRef: "refs/heads/main"}, nil
		default:
			return nil, fmt.Errorf("unexpected %s", method)
		}
	})
	p := NewReviewPane(h.Env, ReviewOptions{Prefix: "chat.", SessionID: "owned"})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 900, 700)
	for range 3 {
		h.Settle(tt)
	}
	return f, h, p, tt
}
func TestNativeDiffUsesTheHostSnapshotAndCanonicalPatchHunk(t *testing.T) {
	f, h, p, tt := reviewSetup(t)
	if p.err != "" || p.patch == nil || len(p.patch.Hunks) != 2 || p.diff == nil {
		t.Fatalf("native patch load failed: %s", p.err)
	}
	if !strings.Contains(p.diff.New.String(), "new3") {
		t.Fatal("the native diff did not display the real patch")
	}
	p.hunk = "1"
	p.requestMutation("stage", false)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mutations) != 1 {
		t.Fatalf("wrong mutation count: %d", len(f.mutations))
	}
	got := f.mutations[0]
	if got["hunk"] != float64(1) || got["revision"] != "snapshot-one" || got["action"] != "stage" || !reflect.DeepEqual(got["paths"], []any{"new.go", "old.go"}) {
		t.Fatalf("mutation lost its source identity: %#v", got)
	}
}
func TestRunningAndWhitespaceFilteredHunksNeverMutate(t *testing.T) {
	f, h, p, tt := reviewSetup(t)
	p.options.Running = func() bool { return true }
	p.requestMutation("revert", false)
	if p.confirm != nil {
		t.Fatal("running task offered a destructive confirmation")
	}
	p.options.Running = nil
	p.ignoreWhitespace = true
	p.hunk = "1"
	p.requestMutation("stage", false)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mutations) != 0 {
		t.Fatal("an unstable displayed hunk was applied")
	}
}
func TestReviewDiscardKeepsCapturedPathsAndRevision(t *testing.T) {
	f, h, p, tt := reviewSetup(t)
	p.requestMutation("revert", false)
	if p.confirm == nil {
		t.Fatal("discard executed without confirmation")
	}
	captured := p.confirm
	p.selected = "another-file"
	p.mutate(captured)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mutations) != 1 || !reflect.DeepEqual(f.mutations[0]["paths"], []any{"new.go", "old.go"}) || f.mutations[0]["revision"] != "snapshot-one" {
		t.Fatalf("confirmation followed a new selection: %#v", f.mutations)
	}
}
func TestForcePushRequiresAnExplicitDecisionWithTheExactRemoteLease(t *testing.T) {
	f, h, p, tt := reviewSetup(t)
	p.push(nil)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	n := len(f.pushes)
	first := f.pushes[0]
	f.mu.Unlock()
	if n != 1 || first["confirmForce"] != nil || p.confirm == nil || p.confirm.lease == nil {
		t.Fatal("normal push silently became a force push")
	}
	p.push(p.confirm.lease)
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pushes) != 2 {
		t.Fatalf("force confirmation count: %d", len(f.pushes))
	}
	lease := f.pushes[1]["confirmForce"].(map[string]any)
	if lease["remoteRef"] != "refs/heads/main" || lease["expectedOid"] != strings.Repeat("a", 40) {
		t.Fatalf("wrong remote revision lease: %#v", lease)
	}
}

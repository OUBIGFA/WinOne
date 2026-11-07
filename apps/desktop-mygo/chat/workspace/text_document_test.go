package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

type textFixture struct {
	mu               sync.Mutex
	source, revision string
	readOnly         bool
	writes           []map[string]string
	entered          chan struct{}
	release          chan struct{}
}

func textSetup(t *testing.T, name string) (*textFixture, *testhost.Harness, *TextDocument, *ui.Tester) {
	t.Helper()
	f := &textFixture{source: "first\r\nsecond\r\n", revision: "first-revision"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, f.source)
	}))
	t.Cleanup(server.Close)
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]string
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		if input["id"] != "session" || input["path"] != name {
			return nil, fmt.Errorf("wrong file scope")
		}
		if method == "chat.file.save" {
			f.mu.Lock()
			entered, release := f.entered, f.release
			f.entered, f.release = nil, nil
			f.mu.Unlock()
			if entered != nil {
				close(entered)
				<-release
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch method {
		case "chat.file.preview":
		case "chat.file.save":
			if input["revision"] != f.revision {
				return nil, testhost.Error{Code: "conflict", Message: "File changed on disk"}
			}
			f.writes = append(f.writes, input)
			f.source = input["text"]
			f.revision = fmt.Sprint("saved-", len(f.writes))
		default:
			return nil, fmt.Errorf("unexpected %s", method)
		}
		return FilePreview{Path: name, Name: name, Kind: "text", MIME: "text/plain", URL: server.URL + "/?grant=private-fixture", Size: int64(len(f.source)), Revision: f.revision, ReadOnly: f.readOnly}, nil
	})
	d := NewTextDocument(h.Env, TextDocumentOptions{Prefix: "chat.", SessionID: "session", Path: name})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); d.View(c) }, 700, 550)
	for range 3 {
		h.Settle(tt)
	}
	t.Cleanup(d.Close)
	return f, h, d, tt
}
func TestNativeEditorLoadsAndSavesWithOriginalLineEndings(t *testing.T) {
	f, h, d, tt := textSetup(t, "notes.ts")
	if d.Error() != "" || d.Buffer() == nil || d.Dirty() {
		t.Fatalf("initial native document: %s", d.Error())
	}
	if err := tt.Click("notes.ts"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("changed\ntext\n")
	if !d.Dirty() {
		t.Fatal("native editor did not update its buffer")
	}
	var saved error
	d.Save(func(err error) { saved = err })
	for range 3 {
		h.Settle(tt)
	}
	if saved != nil || d.Dirty() {
		t.Fatalf("save failed or left a dirty baseline: %v %s", saved, d.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 1 || f.writes[0]["text"] != "changed\r\ntext\r\n" {
		t.Fatalf("line endings changed: %#v", f.writes)
	}
}
func TestLateSaveDoesNotOverwriteOrMarkNewerTypingSaved(t *testing.T) {
	f, h, d, tt := textSetup(t, "notes.txt")
	d.Buffer().Set("snapshot being saved\n")
	entered, release := make(chan struct{}), make(chan struct{})
	f.mu.Lock()
	f.entered, f.release = entered, release
	f.mu.Unlock()
	d.Save(nil)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("save did not reach the Host")
	}
	d.Buffer().Set("typed during save\n")
	close(release)
	for range 3 {
		h.Settle(tt)
	}
	if d.Buffer().String() != "typed during save\n" || !d.Dirty() {
		t.Fatal("late receipt replaced or accepted newer typing")
	}
	d.Save(nil)
	for range 3 {
		h.Settle(tt)
	}
	if d.Dirty() {
		t.Fatal("latest explicit save did not complete")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 2 || !strings.Contains(f.source, "typed during save") {
		t.Fatal("second snapshot was not persisted")
	}
}
func TestRevisionConflictKeepsUnsavedTextAndRefusesBlindReload(t *testing.T) {
	f, h, d, tt := textSetup(t, "notes.txt")
	d.Buffer().Set("my unsaved edit")
	f.mu.Lock()
	f.revision = "external-change"
	f.source = "changed by someone else"
	f.mu.Unlock()
	var failure error
	d.Save(func(err error) { failure = err })
	for range 3 {
		h.Settle(tt)
	}
	if failure == nil || !d.Dirty() || d.Buffer().String() != "my unsaved edit" {
		t.Fatal("revision conflict lost its draft")
	}
	d.Reload()
	for range 3 {
		h.Settle(tt)
	}
	if d.Buffer().String() != "my unsaved edit" {
		t.Fatal("reload discarded unsaved edits without a decision")
	}
}
func TestMarkdownAutosaveWorksAfterParentReadTheDirtyState(t *testing.T) {
	f, h, d, tt := textSetup(t, "notes.md")
	d.Buffer().Set("# Edited markdown\n")
	if !d.Dirty() {
		t.Fatal("expected a dirty document")
	}
	d.Observe()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.Settle(tt)
		f.mu.Lock()
		saved := len(f.writes) > 0
		f.mu.Unlock()
		if saved {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("reading Dirty prevented Markdown autosave")
}
func TestClosingPendingSaveCompletesItsCallbackOnce(t *testing.T) {
	f, h, d, tt := textSetup(t, "notes.txt")
	d.Buffer().Set("save while closing")
	entered, release := make(chan struct{}), make(chan struct{})
	f.mu.Lock()
	f.entered, f.release = entered, release
	f.mu.Unlock()
	calls := 0
	var failure error
	d.Save(func(err error) { calls++; failure = err })
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("save did not start")
	}
	d.Close()
	close(release)
	for range 3 {
		h.Settle(tt)
	}
	if calls != 1 || failure != context.Canceled {
		t.Fatalf("pending save callback: count=%d error=%v", calls, failure)
	}
}
func TestPreviewGrantNeverFollowsRedirectsOrAppearsInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid", http.StatusFound)
	}))
	defer server.Close()
	_, err := FetchPreview(context.Background(), server.URL+"/?grant=secret-grant", 100)
	if err == nil || strings.Contains(err.Error(), "secret-grant") {
		t.Fatalf("redirect or grant boundary failed: %v", err)
	}
	if _, err := FetchPreview(context.Background(), "https://example.invalid", 100); err == nil {
		t.Fatal("external document URL accepted as Host preview")
	}
}

package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pdfium"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/kit"
)

// Explicit real Host/native entry. Its Git repository and every file are
// task-local fixtures; no production repository, player or program is touched.
func TestRealHostNativeDocumentConflictReviewPDFAndMedia(t *testing.T) {
	pipe, profile := os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"), os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if pipe == "" || profile == "" {
		t.Skip("explicit real Host/native matrix entry required")
	}
	client, err := host.NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	var work sync.WaitGroup
	var mu sync.Mutex
	var posted []func()
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Spawn: func(fn func()) { work.Add(1); go func() { defer work.Done(); fn() }() }, Post: func(fn func()) { mu.Lock(); posted = append(posted, fn); mu.Unlock() }, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }}
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.CallInto("chat."+method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	directory := filepath.Join(filepath.Dir(profile), "workspace-native")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git %v: %s %v", args, raw, err)
		}
		return string(raw)
	}
	git("init", "-b", "main")
	path := filepath.Join(directory, "notes.txt")
	if err := os.WriteFile(path, []byte("Original\r\nsecond\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "notes.txt")
	git("-c", "user.name=Native fixture", "-c", "user.email=native@example.invalid", "commit", "-m", "fixture baseline")
	var conversation struct{ ID string }
	call("conversation.new", map[string]string{"directory": directory, "title": "Native work-panel matrix"}, &conversation)
	if conversation.ID == "" {
		t.Fatal("real conversation was not created")
	}
	// Review mutations belong to the conversation's live Pi/Host lease, just
	// as they do when the full Chat controller opens the workspace.
	call("open", map[string]string{"id": conversation.ID}, nil)
	defer call("release", map[string]string{"id": conversation.ID}, nil)
	document := NewDocumentPane(env, DocumentOptions{Prefix: "chat.", SessionID: conversation.ID, Path: path})
	defer document.Close()
	view := document.View
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); view(c) }, 1000, 800)
	settle := func() {
		t.Helper()
		for range 100 {
			work.Wait()
			mu.Lock()
			queue := posted
			posted = nil
			mu.Unlock()
			if len(queue) == 0 {
				tt.Frame()
				return
			}
			for _, fn := range queue {
				fn()
			}
			tt.Frame()
		}
		t.Fatal("real native work pane did not settle")
	}
	settle()
	if document.err != "" || document.text == nil || document.text.Error() != "" {
		t.Fatalf("real document failed: %s", document.err)
	}
	// Focus the production editor itself, rather than the identically named
	// document heading, which ui.Tester's text lookup would choose first.
	view = document.text.View
	settle()
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Saved from real native buffer\nsecond\n")
	document.Save(nil)
	settle()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "Saved from real native buffer\r\nsecond\r\n" || document.Dirty() {
		t.Fatalf("native save did not persist original line endings: %q %v", data, err)
	}
	if err := os.WriteFile(path, []byte("External change\r\nsecond\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	document.text.Buffer().Set("Keep my unsaved conflict")
	document.Save(nil)
	settle()
	if !document.Dirty() || document.text.Error() == "" || document.text.Buffer().String() != "Keep my unsaved conflict" {
		t.Fatal("actual revision conflict did not preserve the native buffer")
	}
	panel := NewPanel(env, PanelOptions{Prefix: "chat.", SessionID: conversation.ID, Directory: directory})
	defer panel.Close()
	panel.tabs = []PanelTab{{ID: "file", Kind: "file", Path: path}}
	panel.bodies["file"] = document
	left := false
	panel.RequestLeave(func() { left = true }, nil)
	if left || panel.guard == nil {
		t.Fatal("actual dirty document left without a decision")
	}
	panel.saveGuard()
	settle()
	if left || panel.guard == nil || panel.guard.error == "" {
		t.Fatal("failed actual save bypassed the leave decision")
	}
	panel.finishGuard()
	if !left {
		t.Fatal("explicit discard did not complete the leave receipt")
	}
	data, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(data), "External change") {
		t.Fatal("discard overwrote the actual external file")
	}
	review := NewReviewPane(env, ReviewOptions{Prefix: "chat.", SessionID: conversation.ID, InitialPath: "notes.txt"})
	defer review.Close()
	view = review.View
	settle()
	if review.err != "" || review.patch == nil || !strings.Contains(review.rawPatch, "External change") {
		t.Fatalf("real review did not load its Host patch: %s", review.err)
	}
	review.requestMutation("stage", false)
	settle()
	if !strings.Contains(git("diff", "--cached", "--", "notes.txt"), "External change") {
		t.Fatalf("native stage did not change the fixture index: error=%q snapshot=%+v selected=%q", review.err, review.snapshot, review.selected)
	}
	review.chooseScope("staged", "")
	settle()
	review.requestMutation("unstage", false)
	settle()
	if git("diff", "--cached", "--", "notes.txt") != "" {
		t.Fatal("native unstage did not change the fixture index")
	}
	// Default PDF/media factories read actual Host-issued file capabilities.
	root := os.Getenv("WINONE_NATIVE_UI_REPOSITORY")
	pdfData, err := os.ReadFile(filepath.Join(root, "tests", "native-pdf", "fixtures", "forms-and-links.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(directory, "forms.pdf")
	if err := os.WriteFile(pdfPath, pdfData, 0600); err != nil {
		t.Fatal(err)
	}
	pdfDoc := NewDocumentPane(env, DocumentOptions{Prefix: "chat.", SessionID: conversation.ID, Path: pdfPath})
	defer pdfDoc.Close()
	view = pdfDoc.View
	settle()
	pdf, ok := pdfDoc.body.(*PDFPane)
	if !ok || pdf.err != "" || pdf.bitmap == nil {
		t.Fatalf("actual default PDF failed: %s", pdfDoc.err)
	}
	pdf.query = "PDF"
	pdf.find()
	settle()
	if len(pdf.matches) != 1 {
		t.Fatal("actual PDF text search failed")
	}
	pdf.openForms()
	for _, draft := range pdf.forms {
		if draft.field.Kind == pdfium.FieldText {
			draft.value = "Native real Host form"
		}
	}
	pdf.applyForms()
	settle()
	if pdf.err != "" {
		t.Fatal(pdf.err)
	}
	filled := false
	for _, field := range pdf.page.Fields {
		filled = filled || field.Value == "Native real Host form"
	}
	if !filled {
		t.Fatal("actual PDF form edits did not reach PDFium")
	}
	mediaPath := os.Getenv("WINONE_NATIVE_UI_VIDEO")
	if mediaPath == "" {
		t.Fatal("matrix requires the task-local silent video fixture")
	}
	video, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	videoPath := filepath.Join(directory, "silent.mp4")
	if err := os.WriteFile(videoPath, video, 0600); err != nil {
		t.Fatal(err)
	}
	mediaDoc := NewDocumentPane(env, DocumentOptions{Prefix: "chat.", SessionID: conversation.ID, Path: videoPath})
	defer mediaDoc.Close()
	view = mediaDoc.View
	settle()
	media, ok := mediaDoc.body.(*MediaPane)
	if !ok {
		t.Fatalf("actual default media factory absent: %s", mediaDoc.err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for !media.state.Opened && media.error == "" && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
		settle()
	}
	if !media.state.Opened || media.error != "" || media.bitmap == nil {
		t.Fatalf("actual Host media capability did not decode: %s", media.error)
	}
	mediaDoc.SetVisible(false)
	settle()
	if media.visible || media.state.Playing {
		t.Fatal("actual hidden media did not pause")
	}
	mediaDoc.Close()
	settle()
	t.Log("real Host document save/CAS conflict/leave guard, native Git stage/unstage, PDF search/forms, default media decode/pause/release")
}

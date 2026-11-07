package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

// itemsHost answers the Host methods of the transcript's native items:
// file checks, file actions, turn reviews and attachment uploads, beside
// fakeChat's conversation.
type itemsHost struct {
	*fakeChat
	files   map[string]map[string]any
	review  map[string]any
	actions []string
	applied []map[string]any
	added   [][]any
	upload  string
}

func (f *itemsHost) handle(method string, params json.RawMessage) (any, error) {
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case "chat.file.stat":
		var results []any
		for _, path := range p["paths"].([]any) {
			if file, ok := f.files[path.(string)]; ok {
				results = append(results, map[string]any{"requested": path, "file": file})
			} else {
				results = append(results, map[string]any{"requested": path, "missing": true})
			}
		}
		return map[string]any{"files": results}, nil
	case "chat.file.open", "chat.file.reveal":
		f.actions = append(f.actions, method+" "+p["path"].(string))
		return nil, nil
	case "chat.review.turn":
		return f.review, nil
	case "chat.review.turn.apply":
		f.applied = append(f.applied, p)
		snapshot := f.review["snapshot"].(map[string]any)
		snapshot["workspaceState"] = "undone"
		return snapshot, nil
	case "chat.attachments.upload":
		return map[string]any{"url": f.upload}, nil
	case "chat.attachments.add":
		f.added = append(f.added, p["paths"].([]any))
		var out []any
		for _, path := range p["paths"].([]any) {
			out = append(out, map[string]any{"path": `C:\imported\` + filepath.Base(path.(string)), "name": filepath.Base(path.(string)), "size": 5})
		}
		return map[string]any{"attachments": out}, nil
	}
	f.mu.Unlock()
	defer f.mu.Lock()
	return f.fakeChat.handle(method, params)
}

func file(path string) map[string]any {
	return map[string]any{"path": path, "name": filepath.Base(strings.ReplaceAll(path, `\`, "/")), "size": 10, "modified": 1}
}

// workedTurn is a finished turn that read a log, ran a command, wrote a
// file, planned, and answered naming two files.
func workedTurn(t *testing.T) *itemsHost {
	f := newFakeChat(t)
	plan := map[string]any{"plan": []any{map[string]any{"step": "Read the log", "status": "completed"},
		map[string]any{"step": "Write the notes", "status": "in_progress"}, map[string]any{"step": "Ship", "status": "pending"}}}
	f.snapshot = model.Snapshot{Messages: []model.Message{
		{ID: "u1", Role: "user", Content: model.TextContent("Why does the build fail?"), TurnID: "u1"},
		{ID: "a1", Role: "assistant", TurnID: "u1", Content: model.Content{Parts: []model.Part{
			{Type: "toolCall", ID: "t1", Name: "read", Arguments: map[string]any{"path": "build.log"}},
			{Type: "toolCall", ID: "t2", Name: "bash", Arguments: map[string]any{"command": "go test ./..."}},
			{Type: "toolCall", ID: "t3", Name: "write", Arguments: map[string]any{"path": "notes.md", "content": "# Notes\n"}},
			{Type: "toolCall", ID: "p1", Name: "update_plan", Arguments: plan}}}},
		{ID: "r1", Role: "toolResult", TurnID: "u1", ToolCallID: "t1", ToolName: "read", Content: model.TextContent("error: missing icon")},
		{ID: "r2", Role: "toolResult", TurnID: "u1", ToolCallID: "t2", ToolName: "bash", Content: model.TextContent("ok")},
		{ID: "r3", Role: "toolResult", TurnID: "u1", ToolCallID: "t3", ToolName: "write", Content: model.TextContent("Wrote notes.md")},
		{ID: "r4", Role: "toolResult", TurnID: "u1", ToolCallID: "p1", ToolName: "update_plan", Content: model.TextContent("Plan updated")},
		{ID: "a2", Role: "assistant", TurnID: "u1", Content: model.TextContent("Fixed in `src/main.go:12`, see [the guide](docs/guide.md) and `useState`.")},
	}}
	return &itemsHost{fakeChat: f,
		files: map[string]map[string]any{
			"notes.md":                file(`E:\WinOne\notes.md`),
			`E:\WinOne\src\main.go`:   file(`E:\WinOne\src\main.go`),
			`E:\WinOne\docs\guide.md`: file(`E:\WinOne\docs\guide.md`),
			`E:\WinOne\build.log`:     file(`E:\WinOne\build.log`),
			`E:\WinOne\useState`:      file(`E:\WinOne\useState`)},
		review: map[string]any{"snapshot": map[string]any{"isReversible": true, "root": `E:\WinOne`, "revision": "r1",
			"files": []any{map[string]any{"path": "notes.md", "status": "A"}}},
			"patches": map[string]any{"notes.md": "@@ -0,0 +1,2 @@\n+# Notes\n+more\n"}},
	}
}

func setupItems(t *testing.T, f *itemsHost) (*testhost.Harness, *Chat, *ui.Tester) {
	h, p, tt := setupWith(t, f.fakeChat, f.handle)
	open(t, h, p, tt, "c1")
	p.ui.expanded = map[string]bool{"process:u1": true}
	settle(h, tt)
	return h, p, tt
}

func TestToolRowsNameTheirWorkAndOpenFileSnapshots(t *testing.T) {
	f := workedTurn(t)
	h, p, tt := setupItems(t, f)
	for _, text := range []string{i18n.T("chat.toolVerb_read"), "build.log", "bash", "go test ./...", i18n.T("chat.toolVerb_create"), "notes.md"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	// The plan call is the turn's plan card, not an action row.
	if tt.HasText("update_plan") || !tt.HasText("Write the notes") {
		t.Fatalf("plan shown as %q", tt.Texts())
	}
	// A command opens its input and output.
	if err := tt.Click("bash go test ./..."); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText(i18n.T("chat.toolInput")) || !tt.HasText("ok") {
		t.Fatalf("command details %q", tt.Texts())
	}
	// A read opens what the tool read, and the current file on request.
	p.ui.keepPosition()
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.toolVerb_read") + " build.log"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText(i18n.T("chat.toolSnapshot_read")) || !tt.HasText("error: missing icon") {
		t.Fatalf("dialog %+v snapshot %q", p.ui.items.tool, tt.Texts())
	}
	shot(t, tt, "chat-tool-snapshot")
	if err := tt.Click(i18n.T("chat.openCurrentFile")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if r := p.ui.panelRequest; r == nil || r.File != `E:\WinOne\build.log` || p.ui.items.tool != nil {
		t.Fatalf("panel %+v dialog %v", r, p.ui.items.tool)
	}
}

func TestTheTranscriptEndsWithTheConversationsFilesAndChanges(t *testing.T) {
	f := workedTurn(t)
	h, p, tt := setupItems(t, f)
	// Collect to the latest-four preview; tool rows may still name this file.
	p.ui.expanded["process:u1"] = false
	settle(h, tt)
	changed := i18n.T("chat.turnFilesChanged", "count", 1)
	for _, text := range []string{changed, "+2 −0", i18n.T("chat.turnUndo"), i18n.T("chat.reviewViewChanges")} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	shot(t, tt, "chat-files")
	// The generated file's chip opens it; its row opens the turn's review.
	if err := tt.Click(i18n.T("chat.reviewOpenFile") + ": " + `E:\WinOne\notes.md`); err != nil {
		t.Fatal(err)
	}
	if r := p.ui.panelRequest; r == nil || r.File != `E:\WinOne\notes.md` {
		t.Fatalf("chip opened %+v", r)
	}
	if err := tt.Click(i18n.T("chat.reviewViewChanges")); err != nil {
		t.Fatal(err)
	}
	if r := p.ui.panelRequest; r.Tab != "review" || r.Review == nil || r.Review.TurnID != "u1" {
		t.Fatalf("review %+v", r)
	}
	if err := tt.Click(i18n.T("chat.turnUndo")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.applied) != 1 || f.applied[0]["mutation"].(map[string]any)["action"] != "undo" || !tt.HasText(i18n.T("chat.turnReapply")) {
		t.Fatalf("applied %v texts %q", f.applied, tt.Texts())
	}
	// Each file's context menu has its actions.
	if err := tt.RightClick(i18n.T("chat.reviewOpenFile") + ": " + `E:\WinOne\notes.md`); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(i18n.T("chat.revealFile")); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	settle(h, tt)
	if len(f.actions) != 1 || f.actions[0] != `chat.file.reveal E:\WinOne\notes.md` {
		t.Fatalf("actions %v", f.actions)
	}
}

func TestAnswersOfferTheFilesTheyName(t *testing.T) {
	f := workedTurn(t)
	h, p, tt := setupItems(t, f)
	// Inline code that is not a path stays code.
	if !tt.HasText("src/main.go:12") || !tt.HasText("the guide") {
		t.Fatalf("refs %q", tt.Texts())
	}
	if refs := p.ui.items.refs["u1"].refs; len(refs) != 2 {
		t.Fatalf("an identifier became a file: %+v", refs)
	}
	if err := tt.Click("src/main.go:12"); err != nil {
		t.Fatal(err)
	}
	if r := p.ui.panelRequest; r == nil || r.File != `E:\WinOne\src\main.go` || r.Line != 12 {
		t.Fatalf("opened %+v", r)
	}
	// Shift+click opens with the system.
	if err := tt.ClickWith(ui.Shift, "src/main.go:12"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.actions) != 1 || f.actions[0] != `chat.file.open E:\WinOne\src\main.go` {
		t.Fatalf("actions %v", f.actions)
	}
	// A link in the text opens through the Host's check.
	if err := tt.Click("the guide"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if r := p.ui.panelRequest; r.File != `E:\WinOne\docs\guide.md` || r.Line != 0 {
		t.Fatalf("link opened %+v", r)
	}
	p.openLink("docs/guide.md#L4", "answer:u1")
	settle(h, tt)
	if r := p.ui.panelRequest; r.File != `E:\WinOne\docs\guide.md` || r.Line != 4 {
		t.Fatalf("link opened %+v", r)
	}
	p.openLink("docs/missing.md", "answer:u1")
	settle(h, tt)
	if !tt.HasText(i18n.T("chat.fileLinkMissing")) {
		t.Fatalf("no failure in %q", tt.Texts())
	}
	p.openLink("https://example.com/a", "answer:u1")
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != "https://example.com/a" {
		t.Fatalf("opened %v", urls)
	}
}

func TestThePlanFloatsAboveTheInputWhileItsCardIsOutOfView(t *testing.T) {
	f := workedTurn(t)
	// A long answer pushes the plan card out of view.
	f.snapshot.Messages[len(f.snapshot.Messages)-1].Content = model.TextContent(strings.Repeat("A long paragraph of the answer.\n\n", 60))
	h, p, tt := setupItems(t, f)
	p.view.Running = true
	p.ui.followLatest()
	for range 3 {
		settle(h, tt)
	}
	step := i18n.T("chat.planStep", "current", 2, "total", 3)
	if !tt.HasText(step) {
		t.Fatalf("no reminder in %q", tt.Texts())
	}
	shot(t, tt, "chat-plan-reminder")
	if err := tt.Click(i18n.T("chat.planTitle") + " · " + step); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.planDismiss")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if tt.HasText(step) {
		t.Fatal("the dismissed reminder stayed")
	}
	// Scrolled to the top, the card is in view: no reminder either.
	p.ui.items.planDismissed = ""
	p.ui.follow, p.ui.transcript.Y = false, 0
	for range 3 {
		settle(h, tt)
	}
	if tt.HasText(step) {
		t.Fatal("reminder beside its card")
	}
}

func TestScrollControlsStepThroughRoundsAndFollowTheEnd(t *testing.T) {
	f := workedTurn(t)
	long := strings.Repeat("Context line of the question.\n", 40)
	f.snapshot.Messages = nil
	for i := range 4 {
		id := "u" + string(rune('1'+i))
		f.snapshot.Messages = append(f.snapshot.Messages,
			model.Message{ID: id, Role: "user", TurnID: id, Content: model.TextContent(long)},
			model.Message{ID: "a" + id, Role: "assistant", TurnID: id, Content: model.TextContent("Answer " + id)})
	}
	h, p, tt := setupItems(t, f)
	for range 3 {
		settle(h, tt)
	}
	s := &p.ui.transcript
	if s.MaxY <= 0 || s.MaxY-s.Y > 1 || !p.ui.follow {
		t.Fatalf("not at the end: y %v max %v follow %v", s.Y, s.MaxY, p.ui.follow)
	}
	// Idle frames keep following, so the next reply stays in view.
	for range 5 {
		tt.Frame()
	}
	if !p.ui.follow {
		t.Fatal("following stopped without the reader scrolling")
	}
	n := p.ui.items.controls
	if len(n.targets) != 4 || n.previous != 2 || n.next != -1 || !n.bottom {
		t.Fatalf("controls %+v", n)
	}
	shot(t, tt, "chat-scroll-controls")
	if err := tt.Click(i18n.T("chat.previousTurn")); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		tt.Advance(50e6)
		settle(h, tt)
	}
	if p.ui.follow || s.Y > n.targets[2]+1 || s.Y < n.targets[2]-1 {
		t.Fatalf("jumped to %v, wanted %v (follow %v)", s.Y, n.targets[2], p.ui.follow)
	}
	if err := tt.Click(i18n.T("navigation.scrollToBottom")); err != nil {
		t.Fatal(err)
	}
	for range 60 {
		tt.Advance(50e6)
		settle(h, tt)
	}
	if !p.ui.follow || s.MaxY-s.Y > 1 {
		t.Fatalf("not back at the end: y %v max %v follow %v gliding %v", s.Y, s.MaxY, p.ui.follow, s.Scrolling())
	}
}

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func paste(t *testing.T, h *testhost.Harness, tt *ui.Tester, data transfer.Data) {
	t.Helper()
	tt.SetClipboardData(data)
	if err := tt.Click(i18n.T("chat.message")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyV)
	settle(h, tt)
}

func TestPastingFilesAndPicturesAttachesThem(t *testing.T) {
	f := workedTurn(t)
	var uploaded []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploaded, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"attachment": map[string]any{"path": `C:\imported\image.png`, "name": "image.png", "size": len(uploaded), "mimeType": r.Header.Get("Content-Type")}})
	}))
	defer server.Close()
	f.upload = server.URL
	h, p, tt := setupItems(t, f)
	dir := t.TempDir()
	note, folder := filepath.Join(dir, "note.txt"), filepath.Join(dir, "folder")
	if err := os.WriteFile(note, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	// Explorer's files attach and folders become references.
	files, _ := transfer.FileData(note, folder)
	paste(t, h, tt, files)
	if len(f.added) != 1 || len(f.added[0]) != 1 || f.added[0][0] != note || p.text != "@"+folder || len(p.files) != 1 {
		t.Fatalf("added %v text %q files %v err %q", f.added, p.text, p.files, p.err)
	}
	// A screenshot uploads its bytes to the conversation; text with it
	// stays out of the draft.
	shotPNG := pngBytes(t)
	paste(t, h, tt, transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, shotPNG), transfer.Bytes(transfer.Text, []byte("copied cells")))))
	if !bytes.Equal(uploaded, shotPNG) || len(p.files) != 2 || p.files[1].Path != `C:\imported\image.png` || strings.Contains(p.text, "copied cells") {
		t.Fatalf("upload %d bytes files %v text %q err %q", len(uploaded), p.files, p.text, p.err)
	}
	// Text alone is the input's.
	paste(t, h, tt, transfer.TextData("plain words"))
	if !strings.HasSuffix(p.text, "plain words") || len(p.files) != 2 {
		t.Fatalf("text %q files %v", p.text, p.files)
	}
}

func TestAPictureForANewChatWaitsInAFileUntilRemoved(t *testing.T) {
	f := workedTurn(t)
	h, p, tt := setupWith(t, f.fakeChat, f.handle)
	paste(t, h, tt, transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngBytes(t)))))
	if len(p.files) != 1 || !strings.HasPrefix(p.files[0].Path, pasteFolder()) {
		t.Fatalf("files %v err %q", p.files, p.err)
	}
	kept := p.files[0].Path
	if _, err := os.Stat(kept); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(i18n.T("chat.removeAttachment")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if _, err := os.Stat(kept); !os.IsNotExist(err) || len(p.files) != 0 {
		t.Fatalf("kept %v files %v", err, p.files)
	}
}

// TestItemShots draws the transcript's native items in both themes, both
// languages and two widths when WINONE_SHOTS names a folder.
func TestItemShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	defer func() { dark = false; i18n.SetLocale(i18n.Chinese) }()
	for _, theme := range []bool{false, true} {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			for _, width := range []int{1100, 640} {
				dark = theme
				f := workedTurn(t)
				f.snapshot.Messages[len(f.snapshot.Messages)-1].Content = model.TextContent("Fixed in `src/main.go:12`, see [the guide](docs/guide.md).\n\n" +
					strings.Repeat("The build reads the generated icons first, then compiles the pages.\n\n", 12))
				h, p, tt := setupItems(t, f)
				i18n.SetLocale(locale)
				tt.SetSize(width, 760)
				name := fmt.Sprintf("%s-%s-%d", map[bool]string{false: "light", true: "dark"}[theme], locale, width)
				p.ui.expanded["tool:t2"] = true
				p.ui.keepPosition()
				settle(h, tt)
				shot(t, tt, "items-top-"+name)
				p.ui.followLatest()
				settle(h, tt)
				settle(h, tt)
				shot(t, tt, "items-end-"+name)
				p.view.Running = true
				settle(h, tt)
				settle(h, tt)
				p.ui.items.planOpen = true
				settle(h, tt)
				shot(t, tt, "items-plan-"+name)
				p.view.Running = false
				p.ui.items.planOpen = false
				p.ui.items.tool = &toolDialog{key: "tool:t3", tool: model.Tool{ID: "t3", Name: "edit",
					Args: map[string]any{"path": "notes.md", "edits": []any{map[string]any{"oldText": "# Notes", "newText": "# Release notes\n\nShipped."}}}, Result: "ok"}}
				settle(h, tt)
				shot(t, tt, "items-dialog-"+name)
			}
		}
	}
}

package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

func TestNativeQueueFailedUpdateRetainsTheEditLockAndOriginalDraft(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: "q", RequestID: "q", Role: "user", Pending: true, Queued: true, Content: model.TextContent("Queued words")})
	fail := true
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.queue" {
			var input map[string]any
			_ = json.Unmarshal(raw, &input)
			f.mu.Lock()
			f.queue = append(f.queue, input)
			f.mu.Unlock()
			if input["action"] == "update" && fail {
				return nil, errors.New("UPDATE_FAILED")
			}
			return f.snapshot, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.updateText("Original draft", []model.Attachment{{Name: "original", Path: "original.txt"}}, "c1")
	settle(h, tt)
	p.run(func(done func(error)) { p.beginQueueEdit("q", done) })
	settle(h, tt)
	p.updateText("Changed queued words", []model.Attachment{{Name: "queued", Path: "queued.txt"}}, "c1")
	p.run(func(done func(error)) { p.finishQueueEdit(true, false, done) })
	settle(h, tt)
	if p.queueEdit == nil || p.text != "Changed queued words" || len(f.queue) != 2 || !strings.Contains(p.err, "UPDATE_FAILED") {
		t.Fatal("failed update released the lock or lost the editor")
	}
	fail = false
	p.run(func(done func(error)) { p.finishQueueEdit(true, false, done) })
	settle(h, tt)
	if p.queueEdit != nil || p.text != "Original draft" || len(p.files) != 1 || p.files[0].Path != "original.txt" || len(f.queue) != 4 || f.queue[3]["locked"] != false {
		t.Fatal("successful edit did not restore the original draft after update/unlock")
	}
}

func TestNativeStopPausesQueueBeforeStoppingAndCancelsUnfinishedSend(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	// Hold the production serial preflight barrier without blocking the UI
	// thread. The real named-pipe queue/stop requests must still complete.
	var release func()
	p.draftWrites.run(func(done func()) { release = done })
	p.text = "Send in preflight"
	p.requestSend(model.DeliverySteer)
	p.stopCurrent()
	settle(h, tt)
	if p.stopInFlight {
		t.Fatal("Stop waited for an unrelated draft write")
	}
	p.text = "Newer after stop"
	release()
	settle(h, tt)
	if len(f.prompts) != 0 || p.text != "Send in preflight\n\nNewer after stop" || p.busy {
		t.Fatalf("stopped preflight sent or lost a draft: prompts=%v text=%q", f.prompts, p.text)
	}
	pause, stop := -1, -1
	for index, method := range f.calls {
		if method == "chat.queue" {
			pause = index
		}
		if method == "chat.stop" {
			stop = index
		}
	}
	if pause < 0 || stop <= pause || f.queue[0]["action"] != "pause" {
		t.Fatalf("stop did not first pause the queue: %v", f.calls)
	}
}

func TestNativeActivityPollingStopsWithHiddenSidebarAndRestartsOnce(t *testing.T) {
	f, h, p, tt, k := setup(t)
	if f.called("chat.activity") != 1 {
		t.Fatal("initial sidebar did not request one activity snapshot")
	}
	p.setSidebar(false)
	k.run()
	settle(h, tt)
	if f.called("chat.activity") != 1 || p.activityPolling {
		t.Fatal("hidden sidebar kept polling")
	}
	p.setSidebar(true)
	p.pollActivity()
	settle(h, tt)
	if f.called("chat.activity") != 2 {
		t.Fatal("reopened sidebar duplicated or lost its poll")
	}
}

func TestNativeManualRenameWinsOverInFlightAITitle(t *testing.T) {
	f := newFakeChat(t)
	started, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.title.regenerate" {
			close(started)
			<-release
			return Conversation{ID: "c1", Title: "AI title"}, nil
		}
		if method == "chat.conversation.update" {
			var input struct{ ID, Title string }
			_ = json.Unmarshal(raw, &input)
			f.mu.Lock()
			defer f.mu.Unlock()
			for i := range f.catalog.Conversations {
				if f.catalog.Conversations[i].ID == input.ID {
					f.catalog.Conversations[i].Title = input.Title
					return f.catalog.Conversations[i], nil
				}
			}
		}
		return f.handle(method, raw)
	})
	p.generateTitle(f.catalog.Conversations[0], nil)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("title generation did not start")
	}
	p.updateConversation(f.catalog.Conversations[0], map[string]any{"title": "Manual title"}, func(err error) {
		if err != nil {
			t.Error(err)
		}
	})
	close(release)
	settle(h, tt)
	if p.catalog.conversation("c1").Title != "Manual title" || p.titleRequests["c1"] {
		t.Fatal("AI completion overwrote a manual rename")
	}
}

func TestNativeQuestionChoicesAndCancelKeepTheComposerDraft(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.updateText("Unsent owner draft", nil, "c1")
	p.view.Approvals = []model.Approval{{Kind: "question", ID: "question", Questions: []model.Question{{Question: "Choose format", Options: []string{"Text", "Table"}}, {Question: "Explain the result"}}}}
	settle(h, tt)
	if err := tt.Click("Table"); err != nil {
		t.Fatal(err)
	}
	focusNativeInput(t, tt, "Explain the result")
	tt.Type("Keep it concise")
	if err := tt.Click(i18n.T("chat.questionSubmit")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.answers) != 1 || f.answers[0]["approvalId"] != "question" || p.text != "Unsent owner draft" {
		t.Fatal("question answer reached the wrong request or stole the draft")
	}
	answers := f.answers[0]["answers"].([]any)
	if fmt.Sprint(answers) != "[Table Keep it concise]" {
		t.Fatalf("wrong question answers: %v", answers)
	}
	p.view.Approvals = []model.Approval{{Kind: "question", ID: "cancel-question", Questions: []model.Question{{Question: "Try again"}}}}
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.questionCancel")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.answers) != 2 || f.answers[1]["cancel"] != true || p.text != "Unsent owner draft" {
		t.Fatal("question cancel lost its request identity or draft")
	}
}

func TestNativeUsageAndPermissionControlsUseTheActualProject(t *testing.T) {
	f := newFakeChat(t)
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.project.permission" {
			var input struct {
				ID         string
				Permission model.Permission
			}
			_ = json.Unmarshal(raw, &input)
			if input.ID != "p1" {
				return nil, errors.New("wrong project")
			}
			f.catalog.Projects[0].Permission = input.Permission
			return f.catalog, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.view.Usage = &model.Usage{Input: 1000, Output: 40, CacheRead: 50, TotalTokens: 1090}
	p.view.ContextUsage = &model.ContextUsage{Tokens: 500, ContextWindow: 1000}
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.contextUsage") + " · 50%"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("500 / 1.0k") || !tt.HasText(i18n.T("chat.usageCostUnknown")) {
		t.Fatal("context usage did not report actual counts/unknown cost")
	}
	p.ui.usageOpen = false
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.permissions") + " · " + i18n.T("chat.ask")); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(i18n.T("chat.full")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !p.ui.confirmFull || p.permission() != model.PermissionAsk {
		t.Fatal("full permission was applied without a decision")
	}
	if err := tt.Click(i18n.T("chat.enableFull")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.permission() != model.PermissionBypass {
		t.Fatal("confirmed permission did not reach the project authority")
	}
}

func TestNativeEarlierHistoryRetainsNewerRowsAndFailureIsRetryable(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.HasOlder, f.snapshot.Before = true, "before"
	failed := true
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.state" && strings.Contains(string(raw), "before") {
			if failed {
				return nil, errors.New("HISTORY_FAILED")
			}
			return model.Snapshot{Messages: []model.Message{{ID: "old", Role: "user", Content: model.TextContent("Older question")}, f.snapshot.Messages[0]}}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.loadEarlier()
	settle(h, tt)
	if p.historyLoading || !strings.Contains(p.err, "HISTORY_FAILED") || len(p.view.Messages) != len(f.snapshot.Messages) {
		t.Fatal("history failure mutated or blocked the current transcript")
	}
	failed = false
	p.loadEarlier()
	settle(h, tt)
	if p.historyLoading || p.view.HasOlder || len(p.view.Messages) != len(f.snapshot.Messages)+1 || p.view.Messages[0].ID != "old" {
		t.Fatal("older history duplicated an overlapping row or lost the newer page")
	}
}

func TestNativeFileKeyboardContextMenuAndMiddleActivation(t *testing.T) {
	f := newFakeChat(t)
	h,p,_:=setupWith(t,f,f.handle)
	// Use the production file chip in its own native focus scope. Opening the
	// full workspace intentionally focuses the editor, not the former chip.
	tt:=ui.NewTester(func(c *ui.Context){
		p.ui.services=c.Services()
		chip:=fileChip(c,"generated:notes","notes.md",`E:\WinOne\notes.md`,false,func(menu *ui.Menu){if menu.Item(i18n.T("chat.copyPath")).Chosen(){p.copy(`E:\WinOne\notes.md`)}},func(){p.openFile(`E:\WinOne\notes.md`,0)})
		if chip.Clicked(){p.openFile(`E:\WinOne\notes.md`,0)}
	},400,80)
	tt.Key(0,ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if p.ui.panelRequests != 1 || p.ui.panelRequest == nil || p.ui.panelRequest.File != `E:\WinOne\notes.md` {
		t.Fatalf("keyboard did not activate the generated file: requests=%d target=%+v", p.ui.panelRequests, p.ui.panelRequest)
	}
	tt.Key(ui.Shift, ui.KeyF10)
	if err := tt.ChooseMenuItem(i18n.T("chat.copyPath")); err != nil {
		t.Fatalf("keyboard context menu failed: %v %v", err, tt.Menu())
	}
	if tt.Clipboard() != `E:\WinOne\notes.md` {
		t.Fatal("keyboard menu copied the wrong file")
	}
	activated := 0
	bounds := ui.Rect{W: 100, H: 28}
	for _, event := range []ui.InputEvent{{Kind: ui.InputPointerDown, Button: 2, X: 20, Y: 10}, {Kind: ui.InputPointerUp, Button: 2, X: 20, Y: 10}, {Kind: ui.InputPointerUp, Button: 2, X: 101, Y: 10}, {Kind: ui.InputPointerUp, Button: 0, X: 20, Y: 10}} {
		fileMiddleClick(event, bounds, func() { activated++ })
	}
	if activated != 1 {
		t.Fatal("middle activation duplicated or activated outside the file")
	}
}

// Text and its field can share an accessible label (Markdown questions and
// declarative forms). Focus the actual native input through keyboard traversal,
// not an arbitrary coordinate below a label or a copy of the component.
func focusNativeInput(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	_ = tt.Click(label)
	for range 100 {
		active, _ := tt.TextInputActive()
		if tt.Focused(label) && active {
			return
		}
		tt.Key(0, ui.KeyTab)
	}
	t.Fatalf("native field %q could not receive keyboard focus", label)
}

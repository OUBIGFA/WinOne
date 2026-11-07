package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

// These exercise production native controls. The fixture owns only the Host
// boundary; real Host/Pi receipts are covered separately by the explicit matrix.
func TestNativePublicProseStaysBetweenIndependentWorkGroups(t *testing.T) {
	f := newFakeChat(t)
	stamp := func(v float64) *float64 { return &v }
	f.snapshot.Messages = []model.Message{
		{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Build then verify")},
		{ID: "a", Role: "assistant", TurnID: "u", Timestamp: stamp(1), Content: model.Content{Parts: []model.Part{{Type: "thinking", Thinking: "Inspect first"}}}},
		{ID: "delivery", Role: "assistant", TurnID: "u", Timestamp: stamp(2), Content: model.Content{Parts: []model.Part{{Type: "text", Text: "First deliverable"}, {Type: "toolCall", ID: "verify", Name: "bash", Arguments: map[string]any{"command": "verify"}}}}},
		{ID: "done", Role: "assistant", TurnID: "u", Timestamp: stamp(3), Content: model.TextContent("Final result")},
	}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	if !tt.HasText("First deliverable") || !tt.HasText("Final result") {
		t.Fatalf("folding work hid public replies: %q", tt.Texts())
	}
	p.view.Running = true
	settle(h, tt)
	if !tt.HasText("First deliverable") {
		t.Fatal("streaming moved a delivered reply into a collapsed work group")
	}
}

func TestNativeWorkPreviewShowsLatestFourAndKeepsItsExpansionAcrossRetry(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Inspect")}}
	for i := range 8 {
		stamp := float64(100 + i)
		f.snapshot.Messages = append(f.snapshot.Messages, model.Message{Role: "assistant", TurnID: "u", Timestamp: &stamp, Content: model.Content{Parts: []model.Part{{Type: "toolCall", ID: fmt.Sprint("t", i), Name: "bash", Arguments: map[string]any{"command": fmt.Sprint("step-", i)}}}}})
	}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	p.view.Running = true
	settle(h, tt)
	if tt.HasText("step-3") || !tt.HasText("step-4") || !tt.HasText("step-7") {
		t.Fatalf("the live preview is not the latest four: %q", tt.Texts())
	}
	if err := tt.Click(i18n.T("chat.processWorking")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("step-0") {
		t.Fatal("expanding did not reveal older activities")
	}
	p.view.Operations = []model.Operation{{ID: "retry", TurnID: "u", Kind: "retry", Status: "running", StartedAt: 300}}
	settle(h, tt)
	if !tt.HasText("step-0") || !tt.HasText(i18n.T("chat.retryConnecting")) {
		t.Fatal("retry reset the disclosure or lost its external status")
	}
	if err := tt.Click(i18n.T("chat.processWorking")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if tt.HasText("step-0") || !tt.HasText("step-7") {
		t.Fatal("folding a live group must retain its latest-four preview")
	}
	p.view.Running = false
	settle(h, tt)
	if tt.HasText("step-3") || !tt.HasText("step-4") || !tt.HasText("step-7") {
		t.Fatalf("completion hid or changed the latest-four preview: %q", tt.Texts())
	}
}

func TestNativeInlineEditorPreservesBubbleWidthAndKeyboardDraftProtection(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Hi")}}
	var edits []map[string]any
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.review.turn":
			return map[string]any{}, nil
		case "chat.edit":
			var input map[string]any
			_ = json.Unmarshal(raw, &input)
			edits = append(edits, input)
			return map[string]any{}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.updateText("Keep composer draft", nil, "c1")
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.editMessage")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	input, ok := tt.Find(i18n.T("chat.editMessage"))
	if !ok || input.W > 90 {
		t.Fatalf("short bubble became a full-width editor: %+v", input)
	}
	tt.Key(0, ui.KeyEscape)
	settle(h, tt)
	if p.ui.edit != nil || p.text != "Keep composer draft" || !tt.HasText("Hi") {
		t.Fatal("Escape did not restore the message and preserve the composer")
	}
	if err := tt.Click(i18n.T("chat.editMessage")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type(strings.Repeat("W", 24))
	settle(h, tt)
	grown, _ := tt.Find(i18n.T("chat.editMessage"))
	if grown.W <= input.W {
		t.Fatal("editing more text did not grow the bubble")
	}
	tt.Compose("拼", 1)
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if len(edits) != 0 {
		t.Fatal("composition Enter submitted an edited message")
	}
	tt.Compose("", 0)
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if len(edits) != 1 || p.ui.edit != nil || p.text != "Keep composer draft" {
		t.Fatal("Enter did not submit once without stealing the composer draft")
	}
}

func TestNativeComposerEnterShiftEnterControlEnterAndComposition(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	if err := tt.Click(i18n.T("chat.message")); err != nil {
		t.Fatal(err)
	}
	tt.Type("First")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("Second")
	settle(h, tt)
	if p.text != "First\nSecond" || len(f.prompts) != 0 {
		t.Fatal("Shift+Enter did not keep a multiline draft")
	}
	tt.Compose("拼", 1)
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if len(f.prompts) != 0 {
		t.Fatal("IME composition submitted a prompt")
	}
	tt.Compose("", 0)
	p.view.Running = true
	settle(h, tt)
	tt.Key(ui.Ctrl, ui.KeyEnter)
	settle(h, tt)
	if len(f.prompts) != 1 || f.prompts[0]["deliveryMode"] != "steer" || p.queuedSend != nil {
		t.Fatalf("Ctrl+Enter did not steer immediately: %v", f.prompts)
	}
}

func TestNativeQueueEditingCannotCancelItsLockedRowAndKeyboardCanReorder(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Active")}}
	for i := range 3 {
		id := fmt.Sprint("q", i)
		f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: id, RequestID: id, Role: "user", Pending: true, Queued: true, Content: model.TextContent(fmt.Sprint("Queued ", i))})
	}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	p.updateText("Owner draft", []model.Attachment{{Path: "owner.txt", Name: "owner.txt"}}, "c1")
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.queue_editAria", "index", 1)); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.queueEdit == nil || p.text != "Queued 0" || len(f.queue) != 1 || f.queue[0]["action"] != "lock" {
		t.Fatal("queue edit did not lock before replacing the composer")
	}
	if err := tt.Click(i18n.T("chat.queue_removeAria", "index", 1)); err == nil {
		settle(h, tt)
	}
	if len(f.queue) != 1 {
		t.Fatal("an edited row can still be cancelled behind the editor")
	}
	if err := tt.Click(i18n.T("chat.queue_cancelEdit")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.queueEdit != nil || p.text != "Owner draft" || len(p.files) != 1 || len(f.queue) != 2 || f.queue[1]["locked"] != false {
		t.Fatal("cancel edit did not unlock and restore the original draft/attachments")
	}
	if err := tt.Click(i18n.T("chat.queue_dragAria", "index", 2)); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	settle(h, tt)
	last := f.queue[len(f.queue)-1]
	if len(f.queue) != 5 || last["action"] != "reorder" || last["targetIndex"] != 3.0 {
		t.Fatalf("keyboard queue order was not sent: %v", f.queue)
	}
}

func TestNativeQueueDragLocksThenReordersBeforeUnlocking(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", Content: model.TextContent("Active")}}
	for i := range 3 {
		id := fmt.Sprint("q", i)
		f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: id, RequestID: id, Role: "user", Pending: true, Queued: true, Content: model.TextContent(fmt.Sprint("Queued ", i))})
	}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	handle, _ := tt.Find(i18n.T("chat.queue_dragAria", "index", 1))
	target, _ := tt.Find(i18n.T("chat.queue_dragAria", "index", 3))
	tt.Press(handle.X+handle.W/2, handle.Y+handle.H/2)
	settle(h, tt)
	if len(f.queue) != 1 || f.queue[0]["action"] != "interaction" || f.queue[0]["locked"] != true {
		t.Fatal("drag did not lock before moving")
	}
	tt.Move(target.X+target.W/2, target.Y+target.H+2)
	tt.Release(target.X+target.W/2, target.Y+target.H+2)
	settle(h, tt)
	if len(f.queue) != 3 || f.queue[1]["action"] != "reorder" || f.queue[1]["targetIndex"] != 3.0 || f.queue[2]["action"] != "interaction" || f.queue[2]["locked"] != false {
		t.Fatalf("drag reordered outside its transaction: %v", f.queue)
	}
}

func TestNativeRecoveryAndHistoryGapCloseOnlyThePreviousWork(t *testing.T) {
	at := func(v float64) *float64 { return &v }
	response := &model.Response{Key: "run", Running: true, Entries: []model.Entry{{Kind: "thinking", Key: "old", At: at(100), Part: model.Part{Thinking: "Old island"}}, {Kind: "thinking", Key: "new", At: at(48 * 60 * 60 * 1000), Part: model.Part{Thinking: "Current island"}}}}
	segments := workSegments(response, true)
	if len(segments) != 2 || segments[0].response.Running || !segments[1].response.Running || segments[1].response.StartedAt == nil || *segments[1].response.StartedAt != 48*60*60*1000 {
		t.Fatalf("history gap swallowed an unrelated run: %+v", segments)
	}
	messages := []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Owner instruction")}, {ID: "a", Role: "assistant", TurnID: "u", Content: model.Content{Parts: []model.Part{{Type: "thinking", Thinking: "Interrupted work"}}}}, {ID: "wake", Role: "user", TurnID: "wake", HiddenInput: true, Content: model.TextContent("Recovery must remain hidden")}, {ID: "b", Role: "assistant", TurnID: "wake", Content: model.Content{Parts: []model.Part{{Type: "thinking", Thinking: "Recovered work"}}}}}
	items := model.Project(messages, nil, true, nil)
	if len(items) != 3 || items[0].Kind != "user" || items[1].Response.Running || !items[2].Response.Running {
		t.Fatalf("recovery did not keep separate ownership: %+v", items)
	}
}

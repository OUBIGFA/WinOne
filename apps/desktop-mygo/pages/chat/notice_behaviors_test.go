package chat

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

func TestNativeCanceledQueueGestureUnlocksWithoutReordering(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", Content: model.TextContent("Active")}}
	for i := range 3 {
		id := fmt.Sprint("q", i)
		f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: id, RequestID: id, Role: "user", Pending: true, Queued: true, Content: model.TextContent(fmt.Sprint("Queued ", i))})
	}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	handle, ok := tt.Find(i18n.T("chat.queue_dragAria", "index", 1))
	if !ok {
		t.Fatal("native queue handle is absent")
	}
	target, ok := tt.Find(i18n.T("chat.queue_dragAria", "index", 3))
	if !ok {
		t.Fatal("native target is absent")
	}
	tt.Press(handle.X+handle.W/2, handle.Y+handle.H/2)
	settle(h, tt)
	tt.Move(target.X+target.W/2, target.Y+target.H+2)
	// MyGo's real cancellation path delivers a canceled release even when
	// the system takes the capture; the Host lock still must be released.
	tt.CancelPress()
	settle(h, tt)
	if p.ui.queue.drag != nil || len(f.queue) != 2 || f.queue[0]["action"] != "interaction" || f.queue[0]["locked"] != true || f.queue[1]["action"] != "interaction" || f.queue[1]["locked"] != false {
		t.Fatalf("canceled gesture committed a reorder or kept its lock: %v", f.queue)
	}
}

func TestNativeCanceledMiddleClickDoesNotOpenTheFile(t *testing.T) {
	opened := 0
	tt := ui.NewTester(func(c *ui.Context) {
		fileChip(c, "file", "notes.md", "notes.md", false, nil, func() { opened++ })
	}, 400, 100)
	bounds, ok := tt.Find("notes.md")
	if !ok {
		t.Fatal("native file chip is absent")
	}
	x, y := bounds.X+bounds.W/2, bounds.Y+bounds.H/2
	tt.PressButton(2, x, y)
	tt.CancelPress()
	tt.Frame()
	if opened != 0 {
		t.Fatal("canceled middle press opened the file")
	}
	tt.PressButton(2, x, y)
	tt.ReleaseButton(2, x, y)
	if opened != 1 {
		t.Fatal("ordinary middle release no longer opens exactly once")
	}
}

func TestNativeApprovalWarningCloseKeepsPermissionAndOwnerDraft(t *testing.T) {
	f := newFakeChat(t)
	h, p, _ := setupWith(t, f, f.handle)
	p.activeID, p.selection, p.text = "c1", "c1", "Keep the owner's draft"
	approval := model.Approval{ID: "needs-review", Tool: "write", AutoReviewUnavailable: true, Args: map[string]any{"path": "notes.md", "content": "protected"}}
	p.view.Approvals = []model.Approval{approval}
	tt := ui.NewTester(func(c *ui.Context) { p.approvalCard(c, p.view.Approvals[0]) }, 560, 500)
	if err := tt.Click(i18n.T("notices.dismiss")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if tt.HasText(i18n.T("chat.unavailable")) || len(p.view.Approvals) != 1 || p.view.Approvals[0].ID != approval.ID || len(f.answers) != 0 || p.text != "Keep the owner's draft" {
		t.Fatal("closing information answered the permission or discarded its draft")
	}
	if !tt.HasText(i18n.T("chat.allowOnce")) || !tt.HasText(i18n.T("chat.deny")) {
		t.Fatal("closing information hid the actual permission choices")
	}
}

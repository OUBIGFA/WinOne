package onebot

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

// Cindy's real local-group speaker has activity, not executionStatus. Its
// teammateActivity summary signals interaction; PromptPanel then reads the
// exact waiting request from that speaker's native conversation controller.
func TestNativeGroupReadsActualTeammateActivityAndAnswersTheOwningPermission(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴"), oneBot("peer", "同事")}}
	h, p, _ := setup(t, f)
	var answers []json.RawMessage
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.onebot.groups.get":
			return map[string]any{"group": map[string]any{"id": "group", "name": "Group", "teammateActivity": map[string]string{"phase": "needs-interaction"}, "round": map[string]any{"status": "running", "speakers": []any{map[string]string{"botId": "self", "sessionId": "main", "activity": "reply"}}}}}, nil
		case "chat.onebot.chat.poll":
			return map[string]any{"cursor": 1, "running": true, "status": "running", "approvals": []model.Approval{{ID: "owned-approval", Tool: "write", Args: map[string]any{"path": "owned.txt", "content": "proof"}}}}, nil
		case "chat.onebot.chat.permission":
			answers = append(answers, raw)
			return true, nil
		}
		return f.handle(method, raw)
	})
	g := newGroupPane(p, "group")
	defer g.Close()
	tt := ui.NewTester(g.View, 1000, 900)
	for range 6 {
		h.Settle(tt)
	}
	if !tt.HasText(i18n.T("chat.permissionTitle")) {
		t.Fatalf("actual group interaction did not open its native card: %q", tt.Texts())
	}
	if err := tt.Click(i18n.T("chat.allowOnce") + "  ↵"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if len(answers) != 1 {
		t.Fatal("group did not answer the waiting request")
	}
	var input struct {
		ID, ApprovalID string
		Allow          bool
	}
	_ = json.Unmarshal(answers[0], &input)
	if input.ID != "main" || input.ApprovalID != "owned-approval" || !input.Allow {
		t.Fatalf("group answered a different owner: %s", answers[0])
	}
}

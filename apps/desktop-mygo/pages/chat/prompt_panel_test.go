package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// The group only renders PromptPanel, never Chat.View. Opening its task must
// still reach the real controller and return the permission to that task.
func TestPromptPanelOpensSpeakerTaskAndAnswersItsPermission(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	f := newFakeChat(t)
	approval := model.Approval{ID: "approval", Tool: "read", Args: map[string]any{"path": "note.txt"}}
	f.pages = []map[string]any{{"events": []any{}, "cursor": 0, "running": true, "approvals": []model.Approval{approval}}}
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		return f.handle(strings.Replace(method, "chat.onebot.chat.", "chat.", 1), params)
	})
	panel := NewPromptPanel(h.Env, "c1", BotBinding{ID: "speaker", Name: "Speaker"})
	panel.chat.after = (&ticks{}).after
	defer panel.Close()
	shown := false
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); shown = panel.View(c) }, 640, 480)
	settle(h, tt)
	if panel.chat.activeID != "c1" || !shown {
		t.Fatalf("speaker prompt never opened: %q %v", panel.chat.activeID, shown)
	}
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if len(f.answers) != 1 || f.answers[0]["id"] != "c1" || f.answers[0]["allow"] != true {
		t.Fatalf("wrong speaker receipt: %+v", f.answers)
	}
}

package capture

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	"winone/internal/captureprefs"
	internalhost "winone/internal/host"
)

func TestCanvasShortcutGroupsCoverAllCatalogActions(t *testing.T) {
	groups := groupedCanvasActions()
	seen := map[string]int{}
	for _, group := range groups {
		for _, action := range group.actions {
			seen[action.ID]++
		}
	}
	if len(seen) != len(captureprefs.CanvasActions) {
		t.Fatalf("grouped %d actions, want %d", len(seen), len(captureprefs.CanvasActions))
	}
	for _, action := range captureprefs.CanvasActions {
		if seen[action.ID] != 1 {
			t.Fatalf("action %q grouped %d times", action.ID, seen[action.ID])
		}
	}
}

func TestCanvasShortcutsDisclosureCustomStatusConflictAndReset(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	fake := newFakeHost(t)
	h := testhost.New(t, fake.handle)
	h.Env.Snapshot = func() internalhost.Snapshot {
		return internalhost.Snapshot{Modules: []internalhost.Module{{ID: "capture", State: "disabled"}}}
	}
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 1600)
	h.Settle(tt)
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "shortcuts.record.start":
			return map[string]string{"token": "tok-1"}, nil
		case "shortcuts.record.stop":
			return map[string]any{}, nil
		default:
			return fake.handle(method, params)
		}
	})

	click(t, tt, "快捷键")
	h.Settle(tt)

	// Dual headings and default disclosure state: annotation expanded, others collapsed.
	if !tt.HasText("全局快捷键") || !tt.HasText("截图内快捷键") {
		t.Fatalf("expected dual headings, texts: %q", tt.Texts())
	}
	if !tt.HasText("矩形") || tt.HasText("复制并结束") || tt.HasText("识别文字") {
		t.Fatalf("unexpected initial disclosure state, texts: %q", tt.Texts())
	}
	if !tt.HasText("11 项") || !tt.HasText("6 项") || !tt.HasText("5 项") {
		t.Fatalf("expected group summaries, texts: %q", tt.Texts())
	}

	// Expanding the second group reveals its actions.
	click(t, tt, "编辑与输出")
	if !tt.HasText("复制并结束") {
		t.Fatalf("expected editing group to expand, texts: %q", tt.Texts())
	}

	// Recording a conflicting key (O is Ellipse) reports canvasErr without mutating draft.
	click(t, tt, "R")
	h.Settle(tt)
	tt.Key(0, ui.KeyO)
	tt.Frame()
	h.Settle(tt)
	if !tt.HasText("椭圆与矩形使用了相同的截图快捷键") && !tt.HasText("矩形与椭圆使用了相同的截图快捷键") {
		t.Fatalf("expected conflict error, texts: %q", tt.Texts())
	}
	if p.draft.Value.CanvasShortcuts != nil {
		t.Fatalf("conflict must not mutate draft: %+v", p.draft.Value.CanvasShortcuts)
	}

	// Recording F8 for Rectangle succeeds, updates status, summary, and enables Rail Save/Undo.
	click(t, tt, "R")
	h.Settle(tt)
	tt.Key(0, ui.KeyF8)
	tt.Frame()
	h.Settle(tt)
	if p.canvasErr != "" || p.draft.Value.CanvasShortcuts["rectangle"] != "F8" {
		t.Fatalf("expected F8 saved in draft, err=%q draft=%+v", p.canvasErr, p.draft.Value.CanvasShortcuts)
	}
	if !tt.HasText("已修改 · 默认 R") || !tt.HasText("11 项 · 已改 1") {
		t.Fatalf("expected custom indicator and summary, texts: %q", tt.Texts())
	}
	if h.Env.Rail.Save.Disabled || h.Env.Rail.Undo.Disabled {
		t.Fatalf("expected unified rail Save and Undo enabled, save=%+v undo=%+v", h.Env.Rail.Save, h.Env.Rail.Undo)
	}

	// Clearing Ellipse shows disabled status.
	click(t, tt, "O")
	h.Settle(tt)
	tt.Key(0, ui.KeyDelete)
	tt.Frame()
	h.Settle(tt)
	if !tt.HasText("已停用 · 默认 O") || !tt.HasText("11 项 · 已改 2") {
		t.Fatalf("expected disabled indicator, texts: %q", tt.Texts())
	}

	// Unified Rail Save persists canvasShortcuts and clears dirty state.
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.saved) != 1 {
		t.Fatalf("expected 1 save call, got %d", len(fake.saved))
	}
	savedShortcuts, _ := fake.saved[0]["canvasShortcuts"].(map[string]any)
	if savedShortcuts["rectangle"] != "F8" || savedShortcuts["ellipse"] != "" {
		t.Fatalf("unexpected saved canvasShortcuts: %+v", fake.saved[0]["canvasShortcuts"])
	}
	if !h.Env.Rail.Save.Disabled {
		t.Fatal("expected Save disabled after saving")
	}

	// Reset defaults restores nil map and makes draft dirty again until saved.
	click(t, tt, "恢复默认")
	tt.Frame()
	if p.draft.Value.CanvasShortcuts != nil || tt.HasText("已修改 · 默认 R") {
		t.Fatalf("expected reset to defaults, draft=%+v", p.draft.Value.CanvasShortcuts)
	}
	if h.Env.Rail.Save.Disabled {
		t.Fatal("expected Save enabled after resetting customized shortcuts")
	}
}

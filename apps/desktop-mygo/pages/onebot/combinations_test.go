package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

// These exercise the production controls and receipts, not source-text parity.
// Real service/window evidence is kept separate from this isolated Host fixture.
func TestDefaultModelReplyTargetsCanBeChangedAndSurviveReload(t *testing.T) {
	h, p, tt := setup(t, &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}})
	p.openDefaultModels()
	h.Settle(tt)
	label := bt("bots.globalSettings.showReplyTarget")
	if err := tt.Click(label); err != nil {
		t.Fatalf("default settings lost the reply-target control: %v", err)
	}
	h.Settle(tt)
	if !p.botBinding("self", "main", false).ShowReplyTargets() {
		t.Fatal("the toggle did not reach the conversation binding")
	}
	reloaded := &page{env: h.Env}
	reloaded.loadPreferences()
	if !reloaded.prefs.ShowReplyTarget {
		t.Fatal("the saved reply-target preference was lost on reload")
	}
}

func TestTaskModelOverrideDoesNotClaimItInheritsThePrimary(t *testing.T) {
	h, p, tt := setup(t, &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}})
	p.settings = newProfileSettings(p, p.profiles[0], "model")
	p.settings.draft.Capabilities.Value.TaskModelOverride = &ModelRoute{Model: "Task override", Harness: "pi"}
	for range 3 { h.Settle(tt) }
	if tt.HasText(bt("bots.model.inheritingPrimary")) { t.Fatal("an explicit task route is described as inherited") }
	p.settings.draft.Capabilities.Value.TaskModelOverride = nil
	h.Settle(tt)
	if !tt.HasText(bt("bots.model.inheritingPrimary")) { t.Fatal("the inherited task route lost its explanation") }
}

func TestMemoryEnabledStateNeverClaimsThatMemoryIsDisabled(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.onebot.memory.list" { return []MemorySummary{}, nil }
		return f.handle(method, raw)
	})
	p.settings = newProfileSettings(p, p.profiles[0], "memory")
	p.settings.draft.Capabilities.Value.Memory = true
	for range 3 { h.Settle(tt) }
	if tt.HasText(bt("bots.memory.disabledHint")) { t.Fatal("enabled memory is described as disabled") }
	p.settings.draft.Capabilities.Value.Memory = false
	h.Settle(tt)
	if !tt.HasText(bt("bots.memory.disabledHint")) { t.Fatal("disabled memory lost its behavioral explanation") }
}

func TestDefaultModelsReplacementReleasesThePreviousSettingsPane(t *testing.T) {
	h, p, tt := setup(t, &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}})
	previous := newProfileSettings(p, p.profiles[0], "voice")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	voice := &voiceSettingsPane{settings: previous, previewCancel: cancel}
	previous.panes["voice"] = voice
	p.settings = previous
	p.openDefaultModels()
	h.Settle(tt)
	if !previous.closed || !voice.closed || ctx.Err() != context.Canceled || p.settings == previous || !p.settings.defaultModels {
		t.Fatal("replacing settings with defaults retained the old pane or preview lifetime")
	}
}

func TestRestoringDefaultModelsRetainsTypingWhileTheResetIsPending(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	entered, release := make(chan struct{}), make(chan struct{})
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.onebot.modelChain.reset" {
			close(entered)
			<-release
			return map[string]any{"modelChain": []ModelRoute{{Model: "reset default", Harness: "pi"}}, "isCustomized": false}, nil
		}
		return f.handle(method, raw)
	})
	p.openDefaultModels()
	p.settings.chain = []ModelRoute{{Model: "before reset", Harness: "pi"}}
	p.settings.restoreDefaultModels()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("model reset did not start")
	}
	p.settings.chain[0].Model = "typed during reset"
	close(release)
	h.Settle(tt)
	if p.settings.chain[0].Model != "typed during reset" || !p.settings.Dirty() || p.settings.chainBaseline[0].Model != "reset default" {
		t.Fatal("model reset receipt overwrote or accepted newer route editing")
	}
}

func TestVoiceSaveReceiptKeepsNewerVoiceAndSentenceEdits(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, _ := setup(t, f)
	entered, release := make(chan struct{}), make(chan struct{})
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.onebot.voice.setPreference" {
			close(entered)
			<-release
			return SpeechPreference{Selection: &SpeechSelection{ModelID: "speech", Voice: "captured"}}, nil
		}
		return f.handle(method, raw)
	})
	d := newProfileSettings(p, p.profiles[0], "voice")
	v := newVoiceSettingsPane(d)
	defer v.Close()
	tt := ui.NewTester(v.View, 700, 800)
	h.Settle(tt)
	v.selection = &SpeechSelection{ModelID: "speech", Voice: "captured"}
	v.inherit, v.edited = false, true
	v.Save()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("voice save never started")
	}
	v.selection.Voice = "typed during save"
	v.sentence = false
	close(release)
	h.Settle(tt)
	if !v.Dirty() || v.selection.Voice != "typed during save" || v.sentence {
		t.Fatal("a late voice receipt accepted newer unsaved settings")
	}
	v.Discard()
	if v.Dirty() || v.selection.Voice != "captured" {
		t.Fatal("undo did not restore the actual saved voice")
	}
}

func TestSelectingAnotherIMConnectionImmediatelyClearsOldLaneActions(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	c := &channelsPane{settings: newProfileSettings(p, p.profiles[0], "channels"), selected: "old", lanes: []IMLane{{SessionID: "old-session"}}, groups: []IMGroup{{ChatID: "old-chat"}}}
	c.loadDetails("new")
	if len(c.lanes) != 0 || len(c.groups) != 0 {
		t.Fatal("new connection temporarily exposed old connection's reset/archive/group actions")
	}
	h.Settle(tt)
}

func TestGroupDeletionRequiresConfirmationAndRetainsContentOnFailure(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴"), oneBot("peer", "同事")}}
	h, p, _ := setup(t, f)
	fail := true
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.onebot.groups.get":
			return map[string]any{"group": GroupDetail{GroupSummary: GroupSummary{ID: "owned", Name: "Team", Members: []GroupMember{{BotID: "self"}, {BotID: "peer"}}}}}, nil
		case "chat.onebot.groups.delete":
			var input map[string]string
			_ = json.Unmarshal(raw, &input)
			if input["groupId"] != "owned" {
				return nil, fmt.Errorf("wrong group owner")
			}
			if fail {
				return nil, fmt.Errorf("DELETE_FAILED")
			}
			return map[string]any{"ok": true}, nil
		}
		return f.handle(method, raw)
	})
	g := newGroupPane(p, "owned")
	p.group = g
	g.draft.Text = "Keep my group draft"
	g.settings = groupSettings(GroupSummary{ID: "owned", Name: "Team", Members: []GroupMember{{BotID: "self"}, {BotID: "peer"}}})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); g.View(c) }, 1000, 800)
	h.Settle(tt)
	if err := tt.Click(bt("bots.groupChat.settings.delete")); err != nil {
		t.Fatalf("group settings lost deletion: %v", err)
	}
	h.Settle(tt)
	if len(h.Called("chat.onebot.groups.delete")) != 0 || !g.deleteOpen {
		t.Fatal("opening deletion skipped its explicit decision")
	}
	if err := tt.Click(bt("bots.groupChat.settings.delete")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if g.closed || !g.deleteOpen || g.draft.Text != "Keep my group draft" || g.error == "" {
		t.Fatalf("failed deletion destroyed group/draft/retry: closed=%v open=%v draft=%q error=%q calls=%d", g.closed, g.deleteOpen, g.draft.Text, g.error, len(h.Called("chat.onebot.groups.delete")))
	}
	fail = false
	if err := tt.Click(bt("bots.groupChat.settings.delete")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !g.closed || p.group != nil || p.section != "groups" || len(h.Called("chat.onebot.groups.delete")) != 2 {
		t.Fatal("accepted deletion did not release the group and return to its list")
	}
}

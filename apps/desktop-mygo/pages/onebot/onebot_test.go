package onebot

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// fakeOneBot is the Host's OneBot service as the native page reads it.
type fakeOneBot struct {
	profiles []map[string]any
	threads  []map[string]any
	ensured  []json.RawMessage
	sends    []json.RawMessage
	groups   []any
}

func (f *fakeOneBot) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "capture.ai.configuration.get":
		return map[string]any{"providers": []any{}}, nil
	case "chat.onebot.modelChain.get":
		return map[string]any{"modelChain": []any{}, "isCustomized": false}, nil
	case "chat.onebot.bots.list":
		return f.profiles, nil
	case "chat.onebot.groups.list":
		return map[string]any{"groups": f.groups}, nil
	case "chat.onebot.events":
		return map[string]any{"events": []any{}, "cursor": 1, "reset": false}, nil
	case "chat.onebot.session.open":
		return map[string]any{"id": "main"}, nil
	case "chat.onebot.dmThread.list":
		return f.threads, nil
	case "chat.onebot.dmThread.ensure":
		f.ensured = append(f.ensured, params)
		return map[string]any{"ok": true, "sessionId": "topic"}, nil
	case "chat.onebot.voice.preference":
		return map[string]any{}, nil
	case "chat.onebot.voice.models":
		return []any{}, nil
	case "chat.onebot.messages.learning":
		return map[string]any{"receipts": map[string]any{}}, nil
	case "chat.onebot.chat.catalog":
		return map[string]any{"projects": []any{}, "conversations": []any{
			map[string]any{"id": "main", "title": "小柴", "directory": `C:\bot`},
			map[string]any{"id": "topic", "title": "话题", "directory": `C:\bot`}}}, nil
	case "chat.onebot.chat.open", "chat.onebot.chat.state":
		return map[string]any{"messages": []any{}, "running": false, "status": "idle", "approvals": []any{}}, nil
	}
	return map[string]any{}, nil
}

func setup(t *testing.T, f *fakeOneBot) (*testhost.Harness, *page, *ui.Tester) {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	p.after = func(time.Duration, func()) func() { return func() {} }
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
		p.View(c)
	}, 1100, 760)
	for range 4 {
		h.Settle(tt)
	}
	t.Cleanup(p.Close)
	return h, p, tt
}

func oneBot(id, name string) map[string]any {
	return map[string]any{"id": id, "name": name, "status": "active", "enabled": true, "canonicalSessionId": "main",
		"sessions": []any{map[string]any{"id": "main", "role": "canonical", "status": "active"}}}
}

func TestMentionsExcludeThisTeammateAndInactiveOnes(t *testing.T) {
	inactive := oneBot("off", "停用")
	inactive["enabled"] = false
	paused := oneBot("paused", "暂停")
	paused["status"] = "paused"
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴"), oneBot("peer", "同事"), inactive, paused}}
	_, p, _ := setup(t, f)
	binding := p.botBinding("self", "main", false)
	mentions := binding.Mentions()
	if len(mentions) != 1 || mentions[0].ID != "peer" {
		t.Fatalf("mentions: %+v", mentions)
	}
	if binding.Name != "小柴" || !binding.CanOpenTopics || binding.StartVoice == nil || binding.ReadAloud == nil {
		t.Fatalf("the main chat lost its capabilities: %+v", binding)
	}
	topic := p.botBinding("self", "topic", true)
	if topic.CanOpenTopics || topic.StartVoice != nil || topic.ReadAloud != nil {
		t.Fatal("a topic must not nest topics, calls or read aloud")
	}
	other := p.botBinding("self", "history", false)
	if other.StartVoice != nil {
		t.Fatal("only the main chat carries a call")
	}
}

func TestCompanionIntentOpensItsChatWithoutAnUnrequestedSettingsDrawer(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	h.Env.Open("onebot", core.Intent{Requested: true, Options: json.RawMessage(`{"botId":"self","sessionId":"main"}`)})
	for range 4 { h.Settle(tt) }
	if p.chat == nil || p.chatSession != "main" || p.settings != nil {
		t.Fatalf("chat=%v session=%q settings=%v error=%q", p.chat != nil, p.chatSession, p.settings != nil, p.error)
	}
}

func TestOpeningAChatKeepsTheUnreadPositionUntilTheTailIsRead(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	before := p.prefs.LastRead["self"]
	if before == 0 {
		t.Fatal("first use starts the read position")
	}
	p.openBot("self")
	for range 4 {
		h.Settle(tt)
	}
	if p.chat == nil || p.chatSession != "main" {
		t.Fatalf("chat not open: section %q error %q", p.section, p.error)
	}
	if p.prefs.LastRead["self"] != before {
		t.Fatal("opening must not declare replies read")
	}
	binding := p.botBinding("self", "main", false)
	if binding.UnreadBoundaryAt == nil || *binding.UnreadBoundaryAt != before {
		t.Fatalf("boundary: %v", binding.UnreadBoundaryAt)
	}
	binding.OnReadThrough(before + 10)
	if p.prefs.LastRead["self"] != before+10 {
		t.Fatal("reading the tail advances the position")
	}
	binding.OnReadThrough(before)
	if p.prefs.LastRead["self"] != before+10 {
		t.Fatal("the position never moves back")
	}
}

func TestTopicsListEnsureAndCloseBesideTheChat(t *testing.T) {
	assistant := int64(5_000)
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")},
		threads: []map[string]any{{"sessionId": "topic", "rootClientId": "root", "replyCount": 3, "lastAssistantAt": assistant}}}
	h, p, tt := setup(t, f)
	p.openBot("self")
	for range 4 {
		h.Settle(tt)
	}
	binding := p.botBinding("self", "main", false)
	thread := binding.Thread("root")
	if thread == nil || thread.Replies != 3 || !thread.Unread {
		t.Fatalf("thread: %+v", thread)
	}
	binding.OpenThread("root", "标题")
	for range 4 {
		h.Settle(tt)
	}
	if len(f.ensured) != 1 || p.topic == nil || p.topicSession != "topic" {
		t.Fatalf("topic not open: %s %v %q", f.ensured, p.topicSession, p.topicError)
	}
	p.prefs.LastRead[threadReadKey("topic")] = assistant
	if binding.Thread("root").Unread {
		t.Fatal("a read topic stays unread")
	}
	p.closeTopic()
	if p.topic != nil || p.topicRoot != "" {
		t.Fatal("topic did not close")
	}
}

func TestSystemPauseStopsReadingAndPausesTheCall(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	v := &voiceSession{parent: p, callID: "call", phase: "listening", ready: true, speechText: map[string]string{}}
	cancelled := false
	v.micCancel = func() { cancelled = true }
	p.voice = v
	r := &readAloud{parent: p, messageID: "m", cancel: func() {}}
	p.readAloud = r
	core.PauseVoiceInput()
	h.Settle(tt)
	if !cancelled || !v.blocked || v.phase != "paused" {
		t.Fatalf("the microphone kept running: cancelled %v blocked %v phase %q", cancelled, v.blocked, v.phase)
	}
	if r.messageID != "" {
		t.Fatal("reading kept going")
	}
	if v.closed {
		t.Fatal("a pause must keep the call and its draft")
	}
	v.closed = true
	p.voice = nil
}

func TestSpeechChunksStayBoundedAndKeepEveryWord(t *testing.T) {
	text := ""
	for i := range 80 {
		text += fmt.Sprintf("第%d句话讲一点内容。", i)
	}
	chunks := speechChunks(text, 100)
	joined := ""
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 100 {
			t.Fatalf("chunk too long: %d", len([]rune(chunk)))
		}
		joined += chunk
	}
	if joined != text {
		t.Fatal("chunks lost text")
	}
}

func TestEveryNativeTextHasBothLanguages(t *testing.T) {
	for _, key := range []string{"bots.dmThread.openFailed", "bots.voiceMode.readAloudFailed", "bots.globalSettings.showReplyTarget", "routines.quiet", "routines.sessionMode"} {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			if text := i18n.Translate(locale, "onebotCindy."+key); text == "onebotCindy."+key {
				t.Errorf("%s has no %s text", key, locale)
			}
		}
	}
}

func TestRosterPreviewIsOnePlainLine(t *testing.T) {
	got := markdownPreview("你好，**这里**是 [链接](http://x)\n\n```mermaid\nA-->B\n```\n\n- 第二行")
	if got != "你好，这里是 链接 第二行" {
		t.Fatalf("preview: %q", got)
	}
}

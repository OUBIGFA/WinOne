package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestMentionsLeaveWithExactUTF16Ranges(t *testing.T) {
	mentions := []BotMention{{ID: "a", Name: "Reader"}, {ID: "b", Name: "Reader Pro"}, {ID: "c", Name: "小柴"}, {ID: "d", Name: "Twin"}, {ID: "e", Name: "Twin"}}
	text, references := serializeBotMentions("😀 @小柴看 @Reader Pro, mail x@Reader and @Readers @Twin", mentions)
	const want = "😀 [小柴](cindy://bot/c)看 [Reader Pro](cindy://bot/b), mail x@Reader and @Readers @Twin"
	if text != want {
		t.Fatalf("wire text: %q", text)
	}
	if len(references) != 2 || references[0].BotID != "c" || references[1].BotID != "b" {
		t.Fatalf("references: %+v", references)
	}
	// Ranges count UTF-16 units: the emoji takes two, as the Host reads them.
	if references[0].Start != 3 || references[0].End != 3+utf16Len("[小柴](cindy://bot/c)") {
		t.Fatalf("first range: %+v", references[0])
	}
}

func TestMentionQueryAndInsertion(t *testing.T) {
	if q := activeBotMention("mail me@x", 9); q != nil {
		t.Fatalf("an address became a mention: %+v", q)
	}
	if q := activeBotMention("hello @ab cd", 12); q != nil {
		t.Fatalf("a query must end at whitespace: %+v", q)
	}
	q := activeBotMention("问 @小", 4)
	if q == nil || q.start != 2 || q.query != "小" {
		t.Fatalf("query: %+v", q)
	}
	text, caret := insertBotMention("问 @小 吧", *q, 4, BotMention{Name: " 小  柴 "})
	if text != "问 @小 柴  吧" || caret != 7 {
		t.Fatalf("insert: %q %d", text, caret)
	}
	rows := matchingBotMentions([]BotMention{{ID: "x-reader", Name: "Other"}, {ID: "2", Name: "My Reader"}, {ID: "3", Name: "Reader"}}, "reader")
	if len(rows) != 3 || rows[0].Name != "Reader" || rows[1].Name != "My Reader" || rows[2].Name != "Other" {
		t.Fatalf("rank: %+v", rows)
	}
}

func TestTimelineGroupsAndUnreadBoundary(t *testing.T) {
	boundary := int64(1_000)
	rows := []botTimeRow{
		{key: "user:1", at: 500, role: "user", settled: true},
		{key: "response:1", at: 900, role: "assistant", settled: true},
		{key: "trace", at: 1100, role: "assistant", settled: true, trace: true},
		{key: "response:2", at: 1200, role: "assistant", settled: true},
		{key: "user:2", at: 500 + botTimeGroupWindow, role: "user", settled: true},
		{key: "response:3", at: 500 + botTimeGroupWindow + 1, role: "assistant", settled: false},
	}
	timeline := buildBotTimeline(rows, &boundary)
	if timeline.unread != "response:2" {
		t.Fatalf("a trace or a stream took the line: %q", timeline.unread)
	}
	if timeline.latestReply != 1200 {
		t.Fatalf("a streaming reply counts as read: %d", timeline.latestReply)
	}
	if _, ok := timeline.groups["user:1"]; !ok || len(timeline.groups) != 2 {
		t.Fatalf("groups: %v", timeline.groups)
	}
	if none := buildBotTimeline(rows, nil); none.unread != "" {
		t.Fatalf("no boundary shows no line: %q", none.unread)
	}
}

func TestStarterAndLearningMetadata(t *testing.T) {
	options := botStarterOptions(json.RawMessage(`["一","二"]`))
	if len(options) != 2 || stripStarterOptionList("你好\n\n- 一\n- 二", options) != "你好" {
		t.Fatalf("options: %v", options)
	}
	if botStarterOptions(json.RawMessage(`["一",2]`)) != nil {
		t.Fatal("a malformed list must offer nothing")
	}
	rows := botLearningRows(json.RawMessage(`[{"kind":"skill","key":"a","title":"写作","action":"created"},{"kind":"memory","key":"b","title":"偏好","action":"updated"},{"kind":"memory","key":"c","title":"地址","action":"created"},{"kind":"x"}]`))
	if len(rows) != 2 || rows[0].Title != "偏好 · 地址" || rows[0].Action != "updated" || rows[1].Kind != "skill" {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestSpokenAnswersNeverGuess(t *testing.T) {
	labels := []string{"确认", "稍后", "明天和后天"}
	cases := map[string]string{
		"确认。":     "select", // An option named 确认 remains a real option.
		"第二个":     "select",
		"明天和后天":   "select",
		"不要选第二个":  "unknown",
		"选项九":     "unknown",
		"第一个或第二个": "ambiguous",
	}
	for text, want := range cases {
		if got := parseVoiceQuestionSelection(text, labels); got.kind != want {
			t.Errorf("%q: %+v, want %s", text, got, want)
		}
	}
	if got := parseVoiceQuestionSelection("确认", []string{"甲"}); got.kind != "confirm" {
		t.Fatalf("confirm: %+v", got)
	}
}

// fakeTeammate adds the OneBot methods a teammate's conversation reads.
func fakeTeammate(f *fakeChat, delegations *[]map[string]any, stops, traces *[]json.RawMessage) testhost.Handler {
	return func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "chat.onebot.messages.learning":
			return map[string]any{"receipts": map[string]any{"a2": []any{map[string]string{"kind": "memory", "key": "m", "title": "偏好", "action": "created"}}}}, nil
		case "chat.onebot.tasks.list":
			return map[string]any{"delegations": *delegations}, nil
		case "chat.onebot.tasks.stop":
			*stops = append(*stops, params)
			return map[string]any{"ok": true}, nil
		case "chat.onebot.tasks.trace":
			*traces = append(*traces, params)
			return map[string]any{}, nil
		case "chat.onebot.tasks.pr.refs":
			return []any{map[string]any{"owner": "o", "repo": "r", "prNumber": 7}}, nil
		case "chat.onebot.tasks.pr.statuses":
			return []any{map[string]any{"ok": true, "owner": "o", "repo": "r", "prNumber": 7, "status": "merged"}}, nil
		}
		return f.handle(method, params)
	}
}

func setupBot(t *testing.T, f *fakeChat, handler testhost.Handler, bot BotBinding) (*testhost.Harness, *Chat, *ui.Tester) {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, handler)
	p := New(h.Env)
	p.bot = &bot
	k := &ticks{}
	p.after = k.after
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
		p.View(c)
	}, 1100, 900)
	settle(h, tt)
	t.Cleanup(p.Close)
	return h, p, tt
}

func TestTeammateConversationQuotesMentionsAndAcknowledges(t *testing.T) {
	f := newFakeChat(t)
	var delegations []map[string]any
	var stops, traces []json.RawMessage
	var read []int64
	stamp := float64(time.Now().UnixMilli())
	for i := range f.snapshot.Messages {
		value := stamp
		f.snapshot.Messages[i].Timestamp = &value
	}
	boundary := int64(0)
	bot := BotBinding{ID: "bot", Name: "小柴", UnreadBoundaryAt: &boundary,
		Mentions:      func() []BotMention { return []BotMention{{ID: "peer", Name: "Reader"}} },
		OnReadThrough: func(at int64) { read = append(read, at) },
	}
	h, p, tt := setupBot(t, f, fakeTeammate(f, &delegations, &stops, &traces), bot)
	if err := tt.Click("Fix the build"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("The **icon** is missing. Run `node") && !tt.HasText("icon") {
		t.Fatalf("reply missing: %q", tt.Texts())
	}
	if !tt.HasText(botT("bots.chat.newMessagesBoundary")) {
		t.Fatalf("unread line missing: %q", tt.Texts())
	}
	if !tt.HasText(botT("bots.learning.memory", "title", "偏好")) {
		t.Fatalf("learning receipt missing: %q", tt.Texts())
	}
	if len(read) == 0 || read[len(read)-1] != int64(stamp) {
		t.Fatalf("the tail did not acknowledge: %v", read)
	}
	if err := tt.Click(botT("chat.messageActionBar.reply")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.botMode.quote == nil {
		t.Fatal("reply did not quote")
	}
	if err := tt.Click(i18n.T("chat.message")); err != nil {
		t.Fatal(err)
	}
	tt.Type("问 @Rea")
	settle(h, tt)
	if !tt.HasText(i18n.T("onebot.mention.action")) {
		t.Fatalf("mention panel: %q", tt.Texts())
	}
	// The input keeps focus; Enter chooses the row instead of sending.
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if p.text != "问 @Reader " || len(f.prompts) != 0 {
		t.Fatalf("mention not inserted: %q prompts %d", p.text, len(f.prompts))
	}
	tt.Key(0, ui.KeyEnter)
	settle(h, tt)
	if len(f.prompts) == 0 {
		t.Fatal("no prompt")
	}
	prompt := f.prompts[len(f.prompts)-1]
	if prompt["text"] != "问 [Reader](cindy://bot/peer) " {
		t.Fatalf("wire: %v", prompt["text"])
	}
	if _, ok := prompt["replyQuote"]; !ok {
		t.Fatalf("quote did not leave: %v", prompt)
	}
	if refs, _ := prompt["agentReferences"].([]any); len(refs) != 1 {
		t.Fatalf("references: %v", prompt["agentReferences"])
	}
	if p.botMode.quote != nil {
		t.Fatal("an accepted quote must clear")
	}
}

func TestTaskCardFollowsTheLiveRowAndActs(t *testing.T) {
	f := newFakeChat(t)
	collaboration, _ := json.Marshal(map[string]any{"botCollaboration": map[string]any{
		"v": 1, "role": "delegation-request", "delegationId": "d1", "fromBotId": "bot", "fromBotName": "小柴", "toBotId": nil, "toBotName": "", "childSessionId": "c2", "objective": "整理发布说明\n第二行"}})
	f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: "card", Role: "assistant", PresentationOnly: true, AgentMeta: collaboration, Content: model.TextContent("")})
	delegations := []map[string]any{{"id": "d1", "status": "running", "title": "发布说明", "childSessionId": "c2", "createdAt": time.Now().Add(-time.Minute).UnixMilli()}}
	var stops, traces []json.RawMessage
	h, _, tt := setupBot(t, f, fakeTeammate(f, &delegations, &stops, &traces), BotBinding{ID: "bot", Name: "小柴"})
	if err := tt.Click("Fix the build"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	settle(h, tt)
	if !tt.HasText("Plan a trip") {
		t.Fatalf("the child's catalog title must win: %q", tt.Texts())
	}
	if !tt.HasText("PR #7") {
		t.Fatalf("pull request chip: %q", tt.Texts())
	}
	if err := tt.Click(botT("bots.collab.stop")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(stops) != 1 || !strings.Contains(string(stops[0]), `"mode":"cancel"`) {
		t.Fatalf("stop: %s", stops)
	}
	if err := tt.Click(botT("bots.collab.addNote")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	delegations = []map[string]any{{"id": "d1", "status": "completed", "title": "发布说明", "childSessionId": "c2", "createdAt": 1000, "completedAt": 61000, "resultSummary": "已完成整理"}}
	h.Settle(tt)
}

func TestFrozenReceiptAndDirectTraceRender(t *testing.T) {
	f := newFakeChat(t)
	receipt, _ := json.Marshal(map[string]any{"botCollaboration": map[string]any{
		"v": 1, "role": "delegation-result", "delegationId": "d9", "fromBotId": "bot", "fromBotName": "小柴", "toBotId": nil, "toBotName": "", "objective": "x",
		"result": map[string]any{"status": "failed", "title": "导出报告", "text": "", "error": "磁盘已满", "artifacts": []any{}}}})
	direct, _ := json.Marshal(map[string]any{"botDirectMessage": map[string]any{"direction": "sent", "peerBotId": "p", "peerBotName": "同事", "viewerBotId": "bot", "threadId": "th"}})
	empty, _ := json.Marshal(map[string]any{"voiceCall": map[string]any{"callId": "call", "turns": []any{}, "handoffs": []any{}}})
	f.snapshot.Messages = append(f.snapshot.Messages,
		model.Message{ID: "r", Role: "assistant", PresentationOnly: true, AgentMeta: receipt, Content: model.TextContent("")},
		model.Message{ID: "d", Role: "assistant", PresentationOnly: true, AgentMeta: direct, Content: model.TextContent("")},
		model.Message{ID: "v", Role: "assistant", PresentationOnly: true, AgentMeta: empty, Content: model.TextContent("")})
	var delegations []map[string]any
	var stops, traces []json.RawMessage
	h, _, tt := setupBot(t, f, fakeTeammate(f, &delegations, &stops, &traces), BotBinding{ID: "bot", Name: "小柴"})
	if err := tt.Click("Fix the build"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	settle(h, tt)
	if !tt.HasText("导出报告") || !tt.HasText("磁盘已满") {
		t.Fatalf("a receipt keeps its frozen state: %q", tt.Texts())
	}
	label := botT("bots.directMessage.sentTo", "name", "同事")
	if !tt.HasText(label) {
		t.Fatalf("direct trace: %q", tt.Texts())
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	opened := h.Opened()
	if len(opened) == 0 || opened[len(opened)-1] != "onebot" {
		t.Fatalf("direct thread link: %v", opened)
	}
}

func TestRichAnswersRenderNativelyAndSendOnlyOnClick(t *testing.T) {
	f := newFakeChat(t)
	source := "说明\n\n```cindy-ui\n" +
		`{"version":1,"seq":1,"op":"create","id":"days","kind":"range","props":{"label":"天数","min":1,"max":9,"default":3}}` + "\n" +
		`{"seq":2,"op":"create","id":"cost","kind":"stat","props":{"label":"合计","prefix":"¥","value":{"op":"mul","args":[{"ref":"days"},20]}}}` + "\n" +
		`{"seq":3,"op":"create","id":"bars","kind":"chart","props":{"label":"分布","data":[{"label":"甲","value":2},{"label":"乙","value":{"ref":"days"}}]}}` + "\n" +
		`{"seq":4,"op":"create","id":"line","kind":"chart","props":{"label":"走势","variant":"line","data":[{"label":"一","value":1},{"label":"二","value":4}]}}` + "\n" +
		`{"seq":5,"op":"create","id":"pic","kind":"svg","props":{"label":"图形","viewBox":[0,0,10,10],"shapes":[{"type":"path","d":"M0 0L5 5A2 2 0 0 1 9 9Z","fill":"danger"},{"type":"text","x":1,"y":9,"text":"点","fontSize":2}]}}` + "\n" +
		`{"seq":6,"op":"create","id":"go","kind":"button","props":{"label":"发送计划","send":{"op":"concat","args":["计划 ",{"ref":"days"}," 天"]}}}` + "\n" +
		"```\n"
	f.snapshot.Messages[len(f.snapshot.Messages)-1].Content = model.TextContent(source)
	h, p, tt := setupWith(t, f, f.handle)
	p.richMemory.loaded = false
	if err := tt.Click("Fix the build"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	for _, want := range []string{"¥60", "分布", "走势", "计划 3 天"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing: %q", want, tt.Texts())
		}
	}
	if tt.HasText(`"seq":1`) {
		t.Fatal("the operations are listed as code")
	}
	before := len(f.prompts)
	settle(h, tt)
	if len(f.prompts) != before {
		t.Fatal("an answer sent a message by itself")
	}
	if err := tt.Click("发送计划"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.prompts) != before+1 || f.prompts[before]["text"] != "计划 3 天" {
		t.Fatalf("click sends the visible text: %v", f.prompts)
	}
	if !tt.HasText(botT("richAnswer.sent")) {
		t.Fatalf("receipt: %q", tt.Texts())
	}
	if err := tt.Click(botT("richAnswer.sent")); err == nil {
		settle(h, tt)
		if len(f.prompts) != before+1 {
			t.Fatal("an accepted submission repeated")
		}
	}
}

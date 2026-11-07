package chat

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Native port of Cindy's teammate conversation (simplifiedBotConversation):
// chat-bot-mode.tsx, chat-bot-mentions.tsx, botDmQuote.ts, BotChatQuoteViews,
// BotLearningFooter and botTimeline.ts. Tracked as chat-bot-mode in
// third_party/cindy/native-ports.json. The Chat controller stays the only
// owner of the transcript, draft and sends; a binding only adds to them.

// BotMention is a teammate the composer's @ panel offers.
type BotMention struct{ ID, Name, Description string }

// BotThread is the topic opened on a message of the main chat.
type BotThread struct {
	SessionID string
	Replies   int
	Unread    bool
}

// BotReadAloud plays one reply at a time; the OneBot page owns its Host lease.
type BotReadAloud struct {
	Available func() bool
	Toggle    func(messageID, text string)
	// State reports whether this message is the one being read, still preparing.
	State func(messageID string) (active, preparing bool)
}

// BotBinding is what the OneBot page gives a teammate's conversation: identity,
// mentions, read state and the navigation Chat cannot do itself.
type BotBinding struct {
	ID, Name string
	Avatar   func(c *ui.Context, size float32)
	Mentions func() []BotMention
	// InvitationStage is the invitation's current stage ("welcome" while the host
	// is still composing a just-invited teammate's first hello).
	InvitationStage func() string
	// UnreadBoundaryAt is the read position captured on entry; nil shows no line.
	UnreadBoundaryAt *int64
	OnReadThrough    func(at int64)
	ReadOnly, Topic  bool
	// CanOpenTopics offers topics on messages; Topic conversations never nest them.
	CanOpenTopics bool
	Thread        func(rootClientID string) *BotThread
	OpenThread    func(rootClientID, title string)
	OpenSettings  func(page string)
	// StartVoice opens a call; nil hides it (read-only chats and topics).
	StartVoice func()
	ReadAloud  *BotReadAloud
	// ActiveCall is the open call, whose live record the saved card must not repeat.
	ActiveCall func() string
	// VoicePanel draws the open call above the composer, so the transcript and any
	// question stay reachable; VoiceBlocked pauses its microphone during an approval.
	VoicePanel   func(c *ui.Context)
	VoiceBlocked func(blocked bool)
	// ShowReplyTargets is 「显示回复对应的消息」.
	ShowReplyTargets func() bool
}

// botReplyQuote is Cindy's ChatReplyQuote wire value.
type botReplyQuote struct {
	ClientID string `json:"clientId"`
	Role     string `json:"role"`
	Excerpt  string `json:"excerpt"`
}

type botAgentReference struct {
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Href  string `json:"href"`
	BotID string `json:"botId"`
	Name  string `json:"name"`
}

// botLearningReceipt is Cindy's BotLearningReceipt.
type botLearningReceipt struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Action string `json:"action"`
}

// botModeState is the conversation's teammate state; it lives with the page, so
// a pending quote survives until it is sent or removed, as Cindy's does per task.
type botModeState struct {
	quote         *botReplyQuote
	learning      map[string]json.RawMessage
	learningRead  uint64
	learningFor   string
	learningAsked string
	mentionFocus  int
	mentionCaret  int
	mentionMoved  bool
	mentionHidden string
	acknowledged  int64
	starterSent   bool
	starterBusy   bool
	timeline      botTimeline
}

func botT(key string, args ...any) string { return i18n.T("onebotCindy."+key, args...) }

// botClientID is Cindy's clientId of a row: the accepted input id of an owner
// message, the transcript entry id otherwise.
func botClientID(m model.Message) string {
	if m.Role == "user" && m.RequestID != "" {
		return m.RequestID
	}
	return m.ID
}

const botQuoteExcerptMax = 280

var (
	botSpaces       = regexp.MustCompile(`\s+`)
	botRichAnswer   = regexp.MustCompile("(?s)```cindy-ui[^\n]*\n.*?(\n```|$)")
	botMentionBreak = regexp.MustCompile(`[\s]`)
)

// botQuoteExcerpt is chatReplyQuoteExcerpt: one line, 280 units at most.
func botQuoteExcerpt(text string) string {
	line := []rune(strings.TrimSpace(botSpaces.ReplaceAllString(text, " ")))
	if len(line) > botQuoteExcerptMax {
		return string(line[:botQuoteExcerptMax-1]) + "…"
	}
	return string(line)
}

// botReadableText is what a quote may show: never the source of a rich answer.
func botReadableText(text string) string { return botRichAnswer.ReplaceAllString(text, "") }

func botQuoteFor(m model.Message) *botReplyQuote {
	id := botClientID(m)
	if id == "" || m.Role != "user" && m.Role != "assistant" {
		return nil
	}
	excerpt := botQuoteExcerpt(botReadableText(m.Text()))
	if excerpt == "" && len(m.Attachments) > 0 {
		excerpt = m.Attachments[0].Name
	}
	return &botReplyQuote{ClientID: id, Role: m.Role, Excerpt: excerpt}
}

// botTimeRow is a visible transcript row, as useBotTimeline reads it.
type botTimeRow struct {
	key     string
	at      int64
	role    string
	settled bool
	trace   bool
}

type botTimeline struct {
	groups      map[string]int64
	unread      string
	latestReply int64
}

const botTimeGroupWindow = 5 * 60 * 1000

// buildBotTimeline ports collectBotMessageTimeGroups and
// findFirstUnreadBotReplyClientId. A reply still streaming is neither unread
// history nor read through yet, and a teammate-to-teammate trace never takes
// the unread line from the next real reply.
func buildBotTimeline(rows []botTimeRow, boundary *int64) botTimeline {
	out := botTimeline{groups: map[string]int64{}}
	start := int64(-1)
	for _, row := range rows {
		if row.at <= 0 {
			continue
		}
		if start < 0 || row.at-start >= botTimeGroupWindow {
			out.groups[row.key] = row.at
			start = row.at
		}
	}
	for _, row := range rows {
		if !row.settled || row.role != "assistant" {
			continue
		}
		out.latestReply = max(out.latestReply, row.at)
		if boundary != nil && out.unread == "" && !row.trace && row.at > *boundary {
			out.unread = row.key
		}
	}
	return out
}

func botTimelineRows(items []*model.Item) []botTimeRow {
	var rows []botTimeRow
	appendRow := func(key string, message *model.Message, settled bool) {
		if message == nil || message.HiddenInput {
			return
		}
		row := botTimeRow{key: key, role: message.Role, settled: settled}
		if message.Timestamp != nil {
			row.at = int64(*message.Timestamp)
		}
		row.trace = len(readBotMeta(*message).BotDirectMessage) > 0
		rows = append(rows, row)
	}
	for _, item := range items {
		if item.Kind == "response" {
			// Delivered prose still belongs to its exact message when the
			// teammate continues with tools; it is not invisible unread history.
			for _, segment := range workSegments(item.Response, true) {
				if segment.prose != nil {
					appendRow("response:"+item.Key+":"+segment.key, segment.prose, segment.prose.ID != "" && !segment.prose.Pending)
				}
			}
			appendRow("response:"+item.Key, item.Response.Answer, !item.Response.Running)
		} else {
			appendRow(item.Kind+":"+item.Key, item.Message, true)
		}
	}
	return rows
}

// botGroupTime is formatBotMessageGroupTime: the time today, otherwise the date too.
func botGroupTime(at int64, current time.Time) string {
	value := time.UnixMilli(at).In(current.Location())
	sameDay := value.Year() == current.Year() && value.YearDay() == current.YearDay()
	switch {
	case sameDay:
		return value.Format("15:04")
	case value.Year() == current.Year():
		return value.Format("01-02 15:04")
	}
	return value.Format("2006-01-02 15:04")
}

// botTimelineMarkers draws the unread line and the centred time above a row.
func (p *Chat) botTimelineMarkers(c *ui.Context, key string) {
	t := c.Theme()
	s := &p.botMode
	if s.timeline.unread == key {
		ui.Row(c.Key("unread")).FillWidth().Gap(8).AlignItems(ui.Center).Label(botT("bots.chat.newMessagesBoundary")).Children(func() {
			ui.Column(c).Grow(1).Height(1).Background(t.Border)
			ui.Text(c, botT("bots.chat.newMessagesBoundary")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
			ui.Column(c).Grow(1).Height(1).Background(t.Border)
		})
	}
	if at, ok := s.timeline.groups[key]; ok {
		ui.Text(c.Key("time"), botGroupTime(at, time.Now())).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).AlignSelf(ui.Center)
	}
}

// botMeta is the agent metadata the teammate views read from a message.
type botMeta struct {
	VoiceCall         json.RawMessage `json:"voiceCall"`
	BotCollaboration  json.RawMessage `json:"botCollaboration"`
	BotDirectMessage  json.RawMessage `json:"botDirectMessage"`
	BotStarterOptions json.RawMessage `json:"botStarterOptions"`
	BotLearning       json.RawMessage `json:"botLearning"`
}

func readBotMeta(m model.Message) botMeta {
	var meta botMeta
	if len(m.AgentMeta) > 0 {
		_ = json.Unmarshal(m.AgentMeta, &meta)
	}
	return meta
}

// botLearningRows is botLearningRows: two quiet rows at most, memory first.
func botLearningRows(raw json.RawMessage) []botLearningReceipt {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	var out []botLearningReceipt
	for _, kind := range []string{"memory", "skill"} {
		var titles []string
		action := "created"
		for _, value := range values {
			var r botLearningReceipt
			if json.Unmarshal(value, &r) != nil || r.Kind != kind || r.Key == "" || r.Title == "" || r.Action != "created" && r.Action != "updated" {
				continue
			}
			titles = append(titles, r.Title)
			if r.Action == "updated" {
				action = "updated"
			}
		}
		if len(titles) > 0 {
			out = append(out, botLearningReceipt{Kind: kind, Key: kind, Title: strings.Join(titles, " · "), Action: action})
		}
	}
	return out
}

// botStarterOptions is readBotStarterOptions: all valid, or none.
func botStarterOptions(raw json.RawMessage) []string {
	var values []any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	var options []string
	for _, value := range values {
		option, ok := value.(string)
		if !ok || strings.TrimSpace(option) == "" || len(utf16.Encode([]rune(option))) > 60 {
			return nil
		}
		options = append(options, option)
	}
	return options[:min(len(options), 4)]
}

// stripStarterOptionList hides the plain list stored for clients without buttons.
func stripStarterOptionList(content string, options []string) string {
	if len(options) == 0 {
		return content
	}
	var list strings.Builder
	list.WriteString("\n\n")
	for i, option := range options {
		if i > 0 {
			list.WriteString("\n")
		}
		list.WriteString("- " + option)
	}
	return strings.TrimSuffix(content, list.String())
}

// loadBotLearning reads the receipts stored beside the transcript. Only the
// newest read may land, and a failed read keeps what is shown.
func (p *Chat) loadBotLearning() {
	if p.bot == nil || p.activeID == "" {
		return
	}
	s := &p.botMode
	s.learningRead++
	request, id := s.learningRead, p.activeID
	call(p.env, botHost("messages.learning"), map[string]any{"sessionId": id}, func(value struct {
		Receipts map[string]json.RawMessage `json:"receipts"`
	}, err error) {
		if err != nil || request != s.learningRead || p.ui.closed {
			return
		}
		s.learning, s.learningFor = value.Receipts, id
	})
}

// BotChanged takes the OneBot events this conversation depends on.
func (p *Chat) BotChanged(kind, sessionID string, reset bool) {
	if reset || kind == "message-meta-changed" && (sessionID == "" || sessionID == p.activeID) {
		p.loadBotLearning()
	}
	if reset || kind == "task-changed" || kind == "lifecycle-changed" {
		p.botTasks.refresh(p)
	}
}

func (p *Chat) botLearningFooter(c *ui.Context, m model.Message) {
	raw := readBotMeta(m).BotLearning
	if s := &p.botMode; s.learningFor == p.activeID {
		if stored, ok := s.learning[m.ID]; ok {
			raw = stored
		}
	}
	for _, row := range botLearningRows(raw) {
		icon, page, key := icons.BookOpen, "capabilities", "bots.learning.skillCreated"
		switch {
		case row.Kind == "memory":
			icon, page, key = icons.Brain, "memory", "bots.learning.memory"
		case row.Action == "updated":
			key = "bots.learning.skillUpdated"
		}
		if kit.GhostButton(c.Key(row.Kind), botT(key, "title", row.Title), icon, false).Clicked() && p.bot.OpenSettings != nil {
			p.bot.OpenSettings(page)
		}
	}
}

// botStarterRow is BotStarterOptions: a choice is the user's own message through
// the normal send path; the row goes away once the user has written anything.
func (p *Chat) botStarterRow(c *ui.Context, options []string, locked bool) {
	s := &p.botMode
	if len(options) == 0 || locked || s.starterSent {
		return
	}
	ui.Row(c.Key("starter")).Wrap().Gap(8).Label(botT("bots.starterOptions")).Children(func() {
		for _, option := range options {
			option := option
			if kit.Button(c.Key(option), option, nil, false).Disabled(s.starterBusy || p.busy).Clicked() {
				s.starterBusy = true
				p.run(func(done func(error)) {
					p.send(&queuedSend{text: option}, model.DeliveryQueue, func(err error) {
						s.starterBusy = false
						s.starterSent = err == nil
						done(err)
					})
				})
			}
		}
	})
}

// botQuoteBlock is the quote above a message: from the original while it is
// loaded here, otherwise from the snapshot taken when the quote was sent.
func (p *Chat) botQuoteBlock(c *ui.Context, quote botReplyQuote, locate bool) {
	t, pal := c.Theme(), kit.Colors(c)
	author := botT("bots.dmThread.you")
	if quote.Role == "assistant" {
		author = p.bot.Name
	}
	excerpt := quote.Excerpt
	for _, m := range p.view.Messages {
		if botClientID(m) == quote.ClientID {
			if text := botQuoteFor(m); text != nil {
				excerpt = text.Excerpt
			}
			break
		}
	}
	lines := func() {
		ui.Column(c).Width(2).FillHeight().Background(t.Border).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, author).FontSize(kit.CaptionSize(c)).FontWeight(500).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Text(c, excerpt).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0)
		})
	}
	// Only a quote whose original can be located is a button.
	if !locate {
		ui.Row(c.Key("quote")).FillWidth().Gap(8).Padding(4, 8).Radius(t.Radius).Background(pal.Panel).Children(lines)
		return
	}
	row := ui.ButtonBase(c.Key("quote")).FillWidth().Justify(ui.Start).Gap(8).Padding(4, 8).Radius(t.Radius).Background(pal.Panel).Label(author + " " + excerpt)
	row.Children(lines)
	if row.Clicked() {
		p.JumpToMessage(quote.ClientID, quote.ClientID)
	}
}

// botReplyTarget is 「回复对应的消息」: above the first loaded reply of a turn, the
// owner message that turn answers, when the setting is on.
func (p *Chat) botReplyTarget(c *ui.Context, m model.Message) {
	if p.bot.ShowReplyTargets == nil || !p.bot.ShowReplyTargets() || m.TurnID == "" {
		return
	}
	for i := len(p.view.Messages) - 1; i >= 0; i-- {
		row := p.view.Messages[i]
		if row.Role == "assistant" && row.TurnID == m.TurnID && strings.TrimSpace(row.Text()) != "" && row.StopReason != "toolUse" {
			if row.ID != m.ID {
				return
			}
			break
		}
	}
	for _, original := range p.view.Messages {
		if original.Role == "user" && !original.HiddenInput && botClientID(original) == m.TurnID {
			if quote := botQuoteFor(original); quote != nil {
				p.botQuoteBlock(c, *quote, true)
			}
			return
		}
	}
}

// botReplyButton puts a quote of this message above the composer.
func (p *Chat) botReplyButton(c *ui.Context, m model.Message) {
	if p.bot.ReadOnly {
		return
	}
	if messageAction(c.Key("reply"), icons.Reply, botT("chat.messageActionBar.reply")).Clicked() {
		if quote := botQuoteFor(m); quote != nil {
			p.botMode.quote = quote
		}
	}
}

// botThreadButton opens or starts the topic of a message.
func (p *Chat) botThreadButton(c *ui.Context, m model.Message) {
	id := botClientID(m)
	if !p.bot.CanOpenTopics || p.bot.OpenThread == nil || id == "" || m.Pending || m.HiddenInput {
		return
	}
	thread := p.bot.Thread(id)
	label := botT("bots.groupChat.server.startTopic")
	if thread != nil {
		label = botT("bots.groupChat.server.openTopic", "count", thread.Replies)
	}
	if kit.IconButton(c.Key("topic"), icons.MessagesSquare, label).Clicked() {
		p.bot.OpenThread(id, botQuoteExcerpt(botReadableText(m.Text())))
	}
}

// botThreadPreview is the line under a message with a topic.
func (p *Chat) botThreadPreview(c *ui.Context, m model.Message) {
	id := botClientID(m)
	if !p.bot.CanOpenTopics || p.bot.Thread == nil || id == "" {
		return
	}
	thread := p.bot.Thread(id)
	if thread == nil {
		return
	}
	label := botT("bots.groupChat.server.replyCount", "count", thread.Replies)
	if thread.Unread {
		label += " · " + botT("bots.dmThread.newReply")
	}
	if kit.GhostButton(c.Key("topic-preview"), label, icons.MessagesSquare, false).Clicked() {
		p.bot.OpenThread(id, botQuoteExcerpt(botReadableText(m.Text())))
	}
}

// botReadAloudButton toggles reading one reply.
func (p *Chat) botReadAloudButton(c *ui.Context, m model.Message, text string, running bool) {
	r := p.bot.ReadAloud
	if r == nil || !r.Available() || m.ID == "" || strings.TrimSpace(text) == "" {
		return
	}
	active, _ := r.State(m.ID)
	label, icon := botT("bots.voiceMode.readAloud"), icons.Volume2
	if active {
		label, icon = botT("bots.voiceMode.stopReadAloud"), icons.Square
	}
	if kit.IconButton(c.Key("read-aloud"), icon, label).Disabled(running).Clicked() {
		r.Toggle(m.ID, text)
	}
}

// botReply puts the teammate's face beside its reply column.
func (p *Chat) botReply(c *ui.Context, avatar bool, body func()) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(28).Shrink(0).Children(func() {
			if avatar && p.bot.Avatar != nil {
				p.bot.Avatar(c, 28)
			}
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(8).Children(body)
	})
}

// botAnswer is a teammate's reply: its face, the message it answers, the text,
// its actions, then the topic, starter choices and what it learned.
func (p *Chat) botAnswer(c *ui.Context, m model.Message, streaming bool, key string, last bool) {
	if key != "" {
		p.botTimelineMarkers(c, "response:"+key)
	}
	if p.botCardFor(c, m) {
		return
	}
	options := botStarterOptions(readBotMeta(m).BotStarterOptions)
	text := stripStarterOptionList(m.Text(), options)
	p.botReply(c, true, func() {
		p.botReplyTarget(c, m)
		p.answerBody(c, m, text, streaming, key)
		if streaming {
			return
		}
		ui.Row(c).Gap(2).Children(func() {
			p.botReadAloudButton(c, m, text, streaming)
			if text != "" && kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
				p.copy(text)
			}
			if !m.Pending {
				p.botReplyButton(c, m)
				p.botThreadButton(c, m)
			}
			if last && kit.IconButton(c, icons.RotateCcw, i18n.T("chat.retry")).Disabled(p.busy || p.view.Running || isPaused(p.view)).Clicked() {
				p.run(func(done func(error)) { p.retryMessage("", done) })
			}
		})
		p.botThreadPreview(c, m)
		if strings.TrimSpace(text) != "" {
			p.botStarterRow(c, options, p.botHasOwnerMessage())
			p.botLearningFooter(c, m)
		}
	})
}

// botHasOwnerMessage is whether the owner has written in this chat.
func (p *Chat) botHasOwnerMessage() bool {
	for _, m := range p.visibleMessages() {
		if m.Role == "user" && !m.HiddenInput && !m.PresentationOnly {
			return true
		}
	}
	return false
}

// botOpeningPending is isBotOpeningPending: the wait for a just-invited
// teammate's first hello, shown only while nothing is in the chat yet.
func (p *Chat) botOpeningPending() bool {
	if p.bot == nil || p.bot.InvitationStage == nil || p.opening || p.activeID == "" {
		return false
	}
	return p.bot.InvitationStage() == "welcome" && len(p.view.Messages) == 0
}

// BotRecheckRead lets the tail acknowledge again when the roster learns of a reply
// after it was read.
func (p *Chat) BotRecheckRead() { p.botMode.acknowledged = 0 }

// botAcknowledge reports the replies read: only the chat's rendered tail counts,
// so history reading and a streaming turn never advance the position.
func (p *Chat) botAcknowledge() {
	s := &p.botMode
	if p.bot == nil || p.bot.OnReadThrough == nil || p.opening || p.view.Running || !p.ui.follow {
		return
	}
	if at := s.timeline.latestReply; at > s.acknowledged {
		s.acknowledged = at
		p.bot.OnReadThrough(at)
	}
}

// Mentions.

// botMentionQuery is the `@query` the caret is in, as rune offsets.
type botMentionQuery struct {
	start int
	query string
}

func botWordBefore(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '@'
}

func botASCIIWord(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'
}

// activeBotMention is activeBotMentionQuery: a query ends at whitespace.
func activeBotMention(text string, caret int) *botMentionQuery {
	runes := []rune(text)
	if caret < 0 || caret > len(runes) {
		return nil
	}
	start := slices.Index(reversed(runes[:caret]), '@')
	if start < 0 {
		return nil
	}
	start = caret - 1 - start
	if start > 0 && botWordBefore(runes[start-1]) {
		return nil
	}
	query := string(runes[start+1 : caret])
	if botMentionBreak.MatchString(query) || caret-start-1 > 64 {
		return nil
	}
	return &botMentionQuery{start: start, query: query}
}

func reversed(runes []rune) []rune {
	out := slices.Clone(runes)
	slices.Reverse(out)
	return out
}

// matchingBotMentions ranks name prefix first, then name or id substring.
func matchingBotMentions(mentions []BotMention, query string) []BotMention {
	wanted := strings.ToLower(query)
	rank := func(m BotMention) int {
		name := strings.ToLower(m.Name)
		switch {
		case strings.HasPrefix(name, wanted):
			return 0
		case strings.Contains(name, wanted):
			return 1
		case strings.Contains(strings.ToLower(m.ID), wanted):
			return 2
		}
		return 3
	}
	var out []BotMention
	for _, m := range mentions {
		if rank(m) < 3 {
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b BotMention) int { return rank(a) - rank(b) })
	return out
}

func botMentionLabel(name string) string {
	return strings.TrimSpace(botSpaces.ReplaceAllString(name, " "))
}

// insertBotMention replaces the typed `@query` with the teammate and a space.
func insertBotMention(text string, query botMentionQuery, caret int, m BotMention) (string, int) {
	runes := []rune(text)
	inserted := []rune("@" + botMentionLabel(m.Name) + " ")
	out := append(append(slices.Clone(runes[:query.start]), inserted...), runes[caret:]...)
	return string(out), query.start + len(inserted)
}

func botReferenceHref(id string) string {
	return "cindy://bot/" + strings.ReplaceAll(url.QueryEscape(id), "+", "%20")
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// serializeBotMentions is Cindy's bot chip wire form: `@Name` of a current
// teammate becomes `[Name](cindy://bot/<id>)` with its exact range in the sent
// text, in UTF-16 units as the Host reads it. Longer names win, and a name two
// teammates share stays text, because the composer cannot tell them apart.
func serializeBotMentions(text string, mentions []BotMention) (string, []botAgentReference) {
	byName := map[string]*BotMention{}
	seen := map[string]bool{}
	for i := range mentions {
		name := botMentionLabel(mentions[i].Name)
		if name == "" {
			continue
		}
		if seen[name] {
			byName[name] = nil
		} else {
			byName[name] = &mentions[i]
		}
		seen[name] = true
	}
	var names []string
	for name, m := range byName {
		if m != nil {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		if diff := len([]rune(b)) - len([]rune(a)); diff != 0 {
			return diff
		}
		return strings.Compare(a, b)
	})
	if len(names) == 0 {
		return text, nil
	}
	runes := []rune(text)
	var output strings.Builder
	var references []botAgentReference
	cursor := 0
	for at := 0; at < len(runes); at++ {
		if runes[at] != '@' || at < cursor || at > 0 && botWordBefore(runes[at-1]) {
			continue
		}
		rest := string(runes[at+1:])
		for _, name := range names {
			if !strings.HasPrefix(rest, name) {
				continue
			}
			label := []rune(name)
			end := at + 1 + len(label)
			if botASCIIWord(label[len(label)-1]) && end < len(runes) && botASCIIWord(runes[end]) {
				continue
			}
			m := byName[name]
			href := botReferenceHref(m.ID)
			escaped := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(name)
			wire := "[" + escaped + "](" + href + ")"
			output.WriteString(string(runes[cursor:at]))
			start := utf16Len(output.String())
			references = append(references, botAgentReference{Kind: "bot", Start: start, End: start + utf16Len(wire), Href: href, BotID: m.ID, Name: name})
			output.WriteString(wire)
			cursor = end
			break
		}
	}
	output.WriteString(string(runes[cursor:]))
	return output.String(), references
}

// botMentionRows is the list for the caret's query, or nothing when dismissed.
func (p *Chat) botMentionRows(caret int) (*botMentionQuery, []BotMention) {
	if p.bot == nil || p.bot.Mentions == nil || p.bot.ReadOnly {
		return nil, nil
	}
	query := activeBotMention(p.text, caret)
	if query == nil || p.botMode.mentionHidden == query.query {
		return nil, nil
	}
	return query, matchingBotMentions(p.bot.Mentions(), query.query)
}

func (p *Chat) chooseBotMention(query botMentionQuery, caret int, m BotMention) {
	text, next := insertBotMention(p.text, query, caret, m)
	p.updateText(text, p.files, p.activeID)
	p.botMode.mentionCaret, p.botMode.mentionMoved, p.botMode.mentionFocus = next, true, 0
}

// botMentionPanel lists teammates above the composer. The input keeps focus; the
// composer handles the arrows, Enter, Tab and Esc.
func (p *Chat) botMentionPanel(c *ui.Context, query *botMentionQuery, caret int, rows []BotMention) {
	if query == nil || len(rows) == 0 {
		return
	}
	t, pal := c.Theme(), kit.Colors(c)
	focus := min(p.botMode.mentionFocus, len(rows)-1)
	ui.Column(c.Key("bot-mentions")).FillWidth().MaxHeight(220).Gap(2).Padding(6).Radius(12).Border(1, t.Border).Background(pal.Panel).Label(i18n.T("onebot.teammates")).Children(func() {
		for i, m := range rows {
			m := m
			row := ui.ButtonBase(c.Key(m.ID)).FillWidth().Justify(ui.Start).Gap(8).Padding(6, 10).Radius(8).AlignItems(ui.Center).Label(m.Name)
			if i == focus {
				row.Background(pal.Selected)
			}
			row.Children(func() {
				ui.Icon(c, icons.Bot).FontSize(16).TextColor(t.TextMuted).Shrink(0)
				name := ui.Text(c, m.Name).SingleLine().Ellipsis("…").MinWidth(0)
				if m.Description != "" {
					name.Tooltip(m.Description)
				}
				ui.Spacer(c)
				ui.Text(c, i18n.T("onebot.mention.action")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).Shrink(0)
			})
			if row.Clicked() {
				p.chooseBotMention(*query, caret, m)
			}
		}
	})
}

// botMentionKey handles a key in the composer while the panel is open.
func (p *Chat) botMentionKey(key ui.Key, query *botMentionQuery, caret int, rows []BotMention) bool {
	if query == nil || len(rows) == 0 {
		return false
	}
	s := &p.botMode
	switch key {
	case ui.KeyDown:
		s.mentionFocus = (min(s.mentionFocus, len(rows)-1) + 1) % len(rows)
	case ui.KeyUp:
		s.mentionFocus = (min(s.mentionFocus, len(rows)-1) + len(rows) - 1) % len(rows)
	case ui.KeyEnter, ui.KeyTab:
		p.chooseBotMention(*query, caret, rows[min(s.mentionFocus, len(rows)-1)])
	case ui.KeyEscape:
		s.mentionHidden = query.query
	default:
		return false
	}
	return true
}

// botComposerPlaceholder is the teammate's prompt: Latin names take spaces in a
// CJK sentence, Chinese names do not, so the catalog carries both shapes.
func botComposerPlaceholder(name string) string {
	latin := strings.TrimSpace(name) != ""
	for _, r := range strings.TrimSpace(name) {
		latin = latin && r >= 0x20 && r <= 0x7e
	}
	if latin {
		return botT("bots.chat.composerPlaceholderLatin", "name", name)
	}
	return botT("bots.chat.composerPlaceholder", "name", name)
}

// botQuotePreview is the quote waiting above the composer; removing it keeps
// whatever was typed.
func (p *Chat) botQuotePreview(c *ui.Context) {
	if p.bot == nil || p.bot.ReadOnly || p.botMode.quote == nil {
		return
	}
	ui.Row(c.Key("bot-quote-preview")).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { p.botQuoteBlock(c, *p.botMode.quote, false) })
		if kit.IconButton(c, icons.X, botT("bots.groupChat.quote.remove")).Clicked() {
			p.botMode.quote = nil
		}
	})
}

// voiceSelection is the outcome of a spoken answer to a question.
type voiceSelection struct {
	kind    string // select, ambiguous, confirm, cancel or unknown
	indices []int
}

var (
	voiceEndPunctuation = regexp.MustCompile(`[。.!！?？]+$`)
	voiceConfirm        = regexp.MustCompile(`^(确认|确认选择|确认提交|提交选择|確認|確認選擇|確認提交|confirm|confirmselection|submit)$`)
	voiceCancel         = regexp.MustCompile(`^(取消|取消选择|重新选择|取消選擇|重新選擇|cancel|cancelselection)$`)
	voiceLead           = regexp.MustCompile(`^(我选择|我要选|我选|选择|选|我選擇|我要選|我選|選擇|選|select|choose)`)
	voiceSeparators     = regexp.MustCompile(`或者|还是|或|和|以及|还有|、|，|,|and|or`)
	voiceAlternative    = regexp.MustCompile(`或者|还是|或|or`)
	voiceOrdinal        = regexp.MustCompile(`^(?:第([一二两三四五六七八九十\d]+)(?:个|项|個|項)?|(?:选项|選項|option)([一二两三四五六七八九十\d]+)|([一二两三四五六七八九十\d]+))$`)
)

func voiceNormalize(value string) string {
	value = strings.ToLower(value)
	value = voiceEndPunctuation.ReplaceAllString(value, "")
	return strings.TrimSpace(botSpaces.ReplaceAllString(value, ""))
}

func voiceOrdinalIndex(value string) int {
	for i, word := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"} {
		if value == word || value == "option"+word {
			return i
		}
	}
	for i, word := range []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth"} {
		if value == word || value == "the"+word || value == word+"option" || value == "the"+word+"option" || value == word+"one" || value == "the"+word+"one" {
			return i
		}
	}
	match := voiceOrdinal.FindStringSubmatch(value)
	if match == nil {
		return -1
	}
	number := first(match[1], match[2], match[3])
	if digits, ok := map[string]int{"一": 1, "二": 2, "两": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}[number]; ok {
		return digits - 1
	}
	n := 0
	for _, r := range number {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n - 1
}

// parseVoiceQuestionSelection is Cindy's whole-utterance grammar: it never
// extracts a choice from a negation, quotation or longer request, and an option
// named "确认" or "取消" remains a real option.
func parseVoiceQuestionSelection(text string, labels []string) voiceSelection {
	normalized := voiceNormalize(text)
	match := func(value string) []int {
		var out []int
		for i, label := range labels {
			if voiceNormalize(label) == value {
				out = append(out, i)
			}
		}
		return out
	}
	single := func(found []int) (voiceSelection, bool) {
		switch len(found) {
		case 0:
			return voiceSelection{}, false
		case 1:
			return voiceSelection{kind: "select", indices: found}, true
		}
		return voiceSelection{kind: "ambiguous"}, true
	}
	if result, ok := single(match(normalized)); ok {
		return result
	}
	if voiceConfirm.MatchString(normalized) {
		return voiceSelection{kind: "confirm"}
	}
	if voiceCancel.MatchString(normalized) {
		return voiceSelection{kind: "cancel"}
	}
	value := voiceLead.ReplaceAllString(normalized, "")
	if result, ok := single(match(value)); ok {
		return result
	}
	parts := voiceSeparators.Split(value, -1)
	var indices []int
	for _, part := range parts {
		found := match(part)
		if len(found) > 1 {
			return voiceSelection{kind: "ambiguous"}
		}
		index := -1
		if len(found) == 1 {
			index = found[0]
		} else {
			index = voiceOrdinalIndex(part)
		}
		if index < 0 || index >= len(labels) {
			return voiceSelection{kind: "unknown"}
		}
		if !slices.Contains(indices, index) {
			indices = append(indices, index)
		}
	}
	if voiceAlternative.MatchString(value) && len(parts) > 1 {
		return voiceSelection{kind: "ambiguous"}
	}
	return voiceSelection{kind: "select", indices: indices}
}

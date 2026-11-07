package onebot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/pages/chat"
	resultui "winone/resultui"
)

// botThreadSummary is Cindy's BotDmThreadSummary.
type botThreadSummary struct {
	SessionID       string `json:"sessionId"`
	RootClientID    string `json:"rootClientId"`
	ReplyCount      int    `json:"replyCount"`
	LastAssistantAt *int64 `json:"lastAssistantAt"`
}

func threadReadKey(sessionID string) string { return "dm-thread:" + sessionID }

// botBinding connects a teammate's conversation to this page. A topic reads and
// writes its own Session but never nests topics, voice or read aloud.
func (p *page) botBinding(botID, sessionID string, topic bool) chat.BotBinding {
	var boundary *int64
	key := botID
	if topic {
		key = threadReadKey(sessionID)
	}
	if at, ok := p.prefs.LastRead[key]; ok {
		boundary = &at
	}
	binding := chat.BotBinding{
		ID: botID, Topic: topic, UnreadBoundaryAt: boundary,
		Avatar: func(c *ui.Context, size float32) {
			if bot := p.bot(botID); bot != nil {
				p.avatars.View(c, *bot, size)
			}
		},
		Mentions: func() []chat.BotMention {
			var out []chat.BotMention
			for _, bot := range p.profiles {
				// Cindy's readBotMention: enabled, active teammates other than this one.
				if bot.ID != botID && bot.Enabled && (bot.Status == "" || bot.Status == "active") {
					out = append(out, chat.BotMention{ID: bot.ID, Name: bot.Name, Description: bot.Description})
				}
			}
			return out
		},
		InvitationStage: func() string {
			if bot := p.bot(botID); bot != nil && bot.Invitation != nil {
				return bot.Invitation.Stage
			}
			return ""
		},
		OnReadThrough: func(at int64) {
			// The roster counts by a transcript entry's time, a little after the
			// message's own: reading the tail covers everything the roster knows.
			if bot := p.bot(botID); !topic && bot != nil && bot.LastMessageAt != nil && *bot.LastMessageAt > at {
				at = *bot.LastMessageAt
			}
			if p.prefs.LastRead[key] < at {
				p.prefs.LastRead[key] = at
				p.savePreferences()
				// The roster's unread count is the Host's, read from these positions.
				p.refresh()
			}
		},
		ShowReplyTargets: func() bool { return p.prefs.ShowReplyTarget },
		OpenSettings:     func(section string) { p.openSettings(botID, section) },
		ActiveCall: func() string {
			if p.voice != nil && !p.voice.closed {
				return p.voice.callID
			}
			return ""
		},
	}
	if bot := p.bot(botID); bot != nil {
		binding.Name = bot.Name
	}
	// A chat just opened may be newer than the roster: only a known different
	// main chat rules it out.
	if main := canonicalOf(p.bot(botID)); topic || main != "" && main != sessionID {
		return binding
	}
	// Only the teammate's main chat carries topics, calls and read aloud.
	binding.CanOpenTopics = true
	binding.Thread = func(root string) *chat.BotThread {
		summary, ok := p.threads[root]
		if !ok {
			return nil
		}
		unread := false
		if summary.LastAssistantAt != nil {
			read, seen := p.prefs.LastRead[threadReadKey(summary.SessionID)]
			unread = !seen || *summary.LastAssistantAt > read
		}
		return &chat.BotThread{SessionID: summary.SessionID, Replies: summary.ReplyCount, Unread: unread}
	}
	binding.OpenThread = func(root, title string) { p.openThread(botID, sessionID, root, title) }
	binding.StartVoice = func() {
		if p.readAloud != nil {
			p.readAloud.stop()
		}
		p.startVoice(botID, sessionID)
	}
	binding.VoicePanel = p.voicePanel
	binding.VoiceBlocked = func(blocked bool) {
		if p.voice != nil {
			p.voice.setBlocked(blocked)
		}
	}
	p.readAloud = newReadAloud(p, botID, sessionID)
	binding.ReadAloud = &chat.BotReadAloud{
		Available: func() bool { return p.readAloud != nil && p.readAloud.available && p.voice == nil },
		Toggle:    func(id, text string) { p.readAloud.toggle(id, text) },
		State: func(id string) (bool, bool) {
			if p.readAloud == nil || p.readAloud.messageID != id {
				return false, false
			}
			return true, p.readAloud.preparing
		},
	}
	return binding
}

func canonicalOf(bot *Profile) string {
	if bot == nil {
		return ""
	}
	return canonical(*bot)
}

// loadThreads reads the topics opened on messages of the main chat. A failed
// read keeps the rows already shown; the next change reads again.
func (p *page) loadThreads() {
	if p.chat == nil || p.chatSession == "" || p.botID == "" {
		return
	}
	p.threadRead++
	request, botID, root := p.threadRead, p.botID, p.chatSession
	oneBotCall(p, "dmThread.list", map[string]string{"botId": botID, "rootSessionId": root}, func(rows []botThreadSummary, err error) {
		if err != nil || request != p.threadRead {
			return
		}
		p.threads = map[string]botThreadSummary{}
		for _, row := range rows {
			p.threads[row.RootClientID] = row
		}
	})
}

// openThread opens the topic beside the main chat, creating it the first time.
func (p *page) openThread(botID, rootSession, rootClient, title string) {
	p.closeTopic()
	p.topicRoot, p.topicBusy, p.topicError = rootClient, true, ""
	p.threadAttempt++
	attempt := p.threadAttempt
	var result struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"errorCode"`
		SessionID string `json:"sessionId"`
	}
	core.Do(p.env, func() error {
		raw, err := p.env.Host.Call("chat.onebot.dmThread.ensure", map[string]any{"botId": botID, "rootSessionId": rootSession, "rootClientId": rootClient, "title": markdownPreview(title)})
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &result)
	}, func(err error) {
		if p.closed || attempt != p.threadAttempt {
			return
		}
		p.topicBusy = false
		switch {
		case err != nil || !result.OK && result.ErrorCode != "ROOT_UNAVAILABLE":
			p.topicError = bt("bots.dmThread.openFailed")
		case !result.OK:
			p.topicError = bt("bots.dmThread.rootUnavailable")
		default:
			p.topicSession = result.SessionID
			p.topic = chat.NewOneBotConversation(p.env, result.SessionID, p.botBinding(botID, result.SessionID, true))
			p.loadThreads()
		}
	})
}

func (p *page) closeTopic() {
	p.threadAttempt++
	if p.topic != nil {
		p.topic.Close()
		p.topic = nil
	}
	p.topicRoot, p.topicSession, p.topicError, p.topicBusy = "", "", "", false
}

// topicView is the panel beside the main chat.
func (p *page) topicView(c *ui.Context, stacked bool) {
	if p.topicRoot == "" {
		return
	}
	pal := kit.Colors(c)
	pane := ui.Column(c.Key("topic"))
	if stacked {
		pane.FillWidth().Grow(1).Basis(0).MinWidth(0).MinHeight(0).BorderWidth(1, 0, 0, 0)
	} else {
		pane.Width(440).Shrink(1).MinWidth(320).FillHeight().BorderWidth(0, 0, 0, 1)
	}
	pane.BorderColor(pal.Hairline).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Padding(8, 12).AlignItems(ui.Center).Children(func() {
			kit.Strong(c, bt("bots.groupChat.mask.catchup.thread")).Grow(1).MinWidth(0)
			if kit.IconButton(c, icons.X, nt("close")).Clicked() {
				p.closeTopic()
				p.loadThreads()
			}
		})
		if p.topicBusy {
			kit.Loader(c)
		}
		kit.InlineError(c, p.topicError)
		if p.topic != nil {
			ui.Column(c).FillWidth().Grow(1).Basis(0).MinHeight(0).Children(func() { p.topic.View(c) })
		}
	})
}

// readAloud plays one reply through the voice lease, as Cindy's
// MessageReadAloudController: bounded requests, one reply at a time.
type readAloud struct {
	parent           *page
	botID, sessionID string
	available        bool
	messageID        string
	preparing        bool
	callID           string
	cancel           context.CancelFunc
}

func newReadAloud(p *page, botID, sessionID string) *readAloud {
	r := &readAloud{parent: p, botID: botID, sessionID: sessionID}
	core.Go(p.env, func() (bool, error) {
		_, err := r.selection()
		return err == nil, nil
	}, func(ok bool, _ error) { r.available = ok })
	return r
}

// selection resolves this teammate's speech model and voice from its settings.
func (r *readAloud) selection() (SpeechSelection, error) {
	p := r.parent
	var preference SpeechPreference
	if err := p.call("voice.preference", map[string]string{"botId": r.botID}, &preference); err != nil {
		return SpeechSelection{}, err
	}
	var models []SpeechModel
	if err := p.call("voice.models", nil, &models); err != nil {
		return SpeechSelection{}, err
	}
	selection, err := resolveSpeechSelection(preference.Selection, models)
	if err != nil {
		return selection, err
	}
	if selection.Voice == "" {
		var catalogue struct {
			Voices []SpeechVoice `json:"voices"`
		}
		if err = p.call("voice.voices", map[string]any{"selection": selection}, &catalogue); err != nil {
			return selection, err
		}
		if len(catalogue.Voices) > 0 {
			selection.Voice = catalogue.Voices[0].ID
		}
	}
	return selection, nil
}

func (r *readAloud) toggle(messageID, text string) {
	if r.messageID == messageID {
		r.stop()
		return
	}
	r.stop()
	ctx, cancel := context.WithCancel(context.Background())
	callID := rand.Text()
	r.messageID, r.preparing, r.callID, r.cancel = messageID, true, callID, cancel
	p := r.parent
	core.Do(p.env, func() error {
		selection, err := r.selection()
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(selection)
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		input["callId"], input["botId"], input["sessionId"] = callID, r.botID, r.sessionID
		if err = p.call("voice.startPlayback", map[string]any{"input": input}, nil); err != nil {
			return err
		}
		// End only this reply's lease, however playback stops.
		defer func() { _ = p.call("voice.end", map[string]string{"callId": callID}, nil) }()
		for i, chunk := range speechChunks(resultui.SpeechText(text, true), 500) {
			if err = ctx.Err(); err != nil {
				return nil
			}
			requestID := callID + ":" + rand.Text()
			var wire SpeechWire
			if err = p.call("voice.speak", map[string]any{"input": map[string]any{"callId": callID, "requestId": requestID, "text": chunk, "streaming": false}}, &wire); err != nil {
				return err
			}
			if i == 0 {
				p.env.Post(func() {
					if r.callID == callID {
						r.preparing = false
					}
				})
			}
			err = playSpeechWire(ctx, wire, func() (PCMChunk, error) {
				var part PCMChunk
				err := p.call("voice.readSpeech", map[string]string{"callId": callID, "requestId": requestID}, &part)
				return part, err
			})
			if err != nil {
				return err
			}
		}
		return nil
	}, func(err error) {
		if r.callID != callID {
			return
		}
		cancel()
		r.messageID, r.preparing, r.callID, r.cancel = "", false, "", nil
		if err != nil && ctx.Err() == nil && !p.closed {
			p.error = bt("bots.voiceMode.readAloudFailed")
		}
	})
}

func (r *readAloud) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.messageID, r.preparing, r.callID, r.cancel = "", false, "", nil
}

// speechChunks is Cindy's speechChunks: split at a sentence or line when one is
// past half the limit, so a long reply is requested in bounded pieces.
func speechChunks(text string, limit int) []string {
	var chunks []string
	runes := []rune(strings.TrimSpace(text))
	for len(runes) > limit {
		end := limit
		for i := limit - 1; i > limit/2; i-- {
			if strings.ContainsRune("。！？\n", runes[i]) || runes[i] == '.' && i+1 < len(runes) && runes[i+1] == ' ' {
				end = i + 1
				break
			}
		}
		if end == limit {
			for i := limit - 1; i > limit/2; i-- {
				if runes[i] == ' ' {
					end = i
					break
				}
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[:end])))
		runes = []rune(strings.TrimSpace(string(runes[end:])))
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

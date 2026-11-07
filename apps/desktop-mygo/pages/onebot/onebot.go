// Package onebot is the native MyGo counterpart of Cindy's local teammates
// route tree. Profile, task, IM, memory, skill and scheduler authorities remain
// in the existing Host. No React page or alternate runtime is created here.
package onebot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/pages/chat"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
)

func init() {
	shell.Register("onebot", core.Conversation, func(env *core.Env) core.Page { return newPage(env) })
}

type preferences struct {
	Section  string           `json:"section"`
	BotID    string           `json:"botId"`
	Sidebar  float32          `json:"sidebar"`
	LastRead map[string]int64 `json:"lastRead"`
	// ShowReplyTarget is Cindy's 「显示回复对应的消息」; off by default.
	ShowReplyTarget bool `json:"showReplyTarget,omitempty"`
}
type page struct {
	env                                                     *core.Env
	profiles                                                []Profile
	groups                                                  []GroupSummary
	providers                                               []widgets.AIProvider
	defaultChain                                            []ModelRoute
	chainCustomized                                         bool
	prefs                                                   preferences
	section, botID, search                                  string
	chat                                                    *chat.Chat
	chatSession                                             string
	group                                                   *groupPane
	groupCreation                                           *groupEditor
	direct                                                  *directPane
	settings                                                *profileSettings
	dictation                                               *dictationSession
	voice                                                   *voiceSession
	creation                                                *creationState
	market                                                  marketState
	importing                                               importState
	lifecycle                                               *lifecycleDecision
	avatars                                                 avatarCache
	loading, loaded, refreshing, refreshAgain, busy, closed bool
	error, notice, eventError                               string
	cursor                                                  float64
	polling                                                 bool
	generation                                              uint64
	after                                                   func(time.Duration, func()) func()
	cancelPoll                                              func()
	cancelVoicePause                                        func()
	readAloud                                               *readAloud
	threads                                                 map[string]botThreadSummary
	threadRead, threadAttempt                               uint64
	topic                                                   *chat.Chat
	topicRoot, topicSession, topicError                     string
	topicBusy                                               bool
	pageBox, contentBox                                     ui.Handle
}
type lifecycleDecision struct {
	bot                         Profile
	action, confirmName         string
	retainWorktree, keepHistory bool
	busy                        bool
	error                       string
}

func newPage(env *core.Env) *page {
	p := &page{env: env, section: "home", cursor: 9007199254740991, loading: true}
	p.after = func(d time.Duration, fn func()) func() {
		timer := time.AfterFunc(d, func() { env.Post(fn) })
		return func() { timer.Stop() }
	}
	p.cancelVoicePause = core.OnVoicePause(func() { env.Post(p.pauseVoiceInput) })
	p.loadPreferences()
	p.section = "home" // Never restore an incomplete mutation or transient dialog.
	p.loadModels()
	p.refresh()
	return p
}
func (p *page) call(method string, params, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	raw, err := p.env.Host.Call("chat.onebot."+method, params)
	if err != nil {
		return err
	}
	var failure struct {
		OK        *bool           `json:"ok"`
		ErrorCode string          `json:"errorCode"`
		Message   string          `json:"message"`
		Error     json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &failure) == nil && failure.OK != nil && !*failure.OK {
		message := failure.Message
		if message == "" {
			_ = json.Unmarshal(failure.Error, &message)
		}
		if message == "" {
			message = nt("operationFailed", "code", failure.ErrorCode)
		}
		return fmt.Errorf("%s", message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
func oneBotCall[T any](p *page, method string, params any, done func(T, error)) {
	core.Go(p.env, func() (T, error) { var value T; err := p.call(method, params, &value); return value, err }, func(value T, err error) {
		if !p.closed {
			done(value, err)
		}
	})
}
func (p *page) loadModels() {
	type result struct {
		providers []widgets.AIProvider
		chain     []ModelRoute
		custom    bool
	}
	core.Go(p.env, func() (result, error) {
		var r result
		var configuration struct {
			Providers []widgets.AIProvider `json:"providers"`
		}
		if err := p.env.Host.CallInto("capture.ai.configuration.get", nil, &configuration); err != nil {
			return r, err
		}
		r.providers = configuration.Providers
		var chain struct {
			ModelChain   []ModelRoute `json:"modelChain"`
			IsCustomized bool         `json:"isCustomized"`
		}
		if err := p.call("modelChain.get", nil, &chain); err != nil {
			return r, err
		}
		r.chain, r.custom = chain.ModelChain, chain.IsCustomized
		return r, nil
	}, func(r result, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		p.providers, p.defaultChain, p.chainCustomized = r.providers, r.chain, r.custom
	})
}
func (p *page) refresh() {
	if p.closed {
		return
	}
	if p.refreshing {
		p.refreshAgain = true
		return
	}
	p.refreshing = true
	reads := copyValue(p.prefs.LastRead)
	type result struct {
		profiles []Profile
		groups   []GroupSummary
	}
	core.Go(p.env, func() (result, error) {
		var r result
		if err := p.call("bots.list", map[string]any{"lastReadAtByBotId": reads, "locale": botLocale()}, &r.profiles); err != nil {
			return r, err
		}
		var groups struct {
			Groups []GroupSummary `json:"groups"`
		}
		if err := p.call("groups.list", nil, &groups); err != nil {
			return r, err
		}
		r.groups = groups.Groups
		return r, nil
	}, func(r result, err error) {
		if p.closed {
			return
		}
		p.refreshing, p.loading = false, false
		defer func() {
			if p.refreshAgain {
				p.refreshAgain = false
				p.refresh()
			}
		}()
		if err != nil {
			p.error = err.Error()
			return
		}
		first := !p.loaded
		p.loaded = true
		p.error = ""
		p.profiles, p.groups = r.profiles, r.groups
		p.avatars.load(p, r.profiles)
		if bot := p.bot(p.botID); p.chat != nil && bot != nil && bot.UnreadCount > 0 {
			p.chat.BotRecheckRead()
		}
		if p.settings != nil {
			if bot := p.bot(p.settings.botID); bot != nil {
				p.settings.reconcile(*bot)
			}
		}
		if first {
			// First native use starts read positions now, rather than declaring
			// every historical reply unread. Existing positions are preserved.
			for _, bot := range r.profiles {
				if _, ok := p.prefs.LastRead[bot.ID]; !ok {
					p.prefs.LastRead[bot.ID] = time.Now().UnixMilli()
				}
			}
			p.savePreferences()
			p.pollEvents()
		}
	})
}
func (p *page) bot(id string) *Profile {
	for index := range p.profiles {
		if p.profiles[index].ID == id {
			return &p.profiles[index]
		}
	}
	return nil
}
func (p *page) mergeProfile(bot Profile) {
	for index := range p.profiles {
		if p.profiles[index].ID == bot.ID {
			p.profiles[index] = bot
			return
		}
	}
	p.profiles = append(p.profiles, bot)
}
func (p *page) mutate(method string, params any, done func(json.RawMessage)) {
	if p.closed || p.busy {
		return
	}
	p.busy = true
	p.error = ""
	oneBotCall(p, method, params, func(raw json.RawMessage, err error) {
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		if done != nil {
			done(raw)
		}
		p.refresh()
	})
}
func (p *page) navigate(section string) {
	p.env.RequestChange(func() {
		p.closeContent()
		if p.settings != nil {
			p.settings.Close()
			p.settings = nil
		}
		p.section = section
		p.error = ""
		p.prefs.Section = section
		p.savePreferences()
	})
}
func (p *page) openBot(id string) {
	bot := p.bot(id)
	if bot == nil {
		return
	}
	if bot.Status != "active" {
		p.openSettings(id, "home")
		return
	}
	p.env.RequestChange(func() {
		p.mutate("session.open", map[string]string{"botId": id}, func(raw json.RawMessage) {
			var conversation struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw, &conversation); err != nil || conversation.ID == "" {
				p.error = nt("sessionMissing")
				return
			}
			p.showConversation(id, conversation.ID)
		})
	})
}
func (p *page) showConversation(botID, sessionID string) {
	p.closeContent()
	p.botID, p.chatSession, p.section = botID, sessionID, "chat"
	// The read position advances from the rendered tail (OnReadThrough), so the
	// unread line can still show what arrived while this chat was closed.
	p.threads = nil
	p.chat = chat.NewOneBotConversation(p.env, sessionID, p.botBinding(botID, sessionID, false))
	p.loadThreads()
	p.prefs.BotID = botID
	p.savePreferences()
}
func (p *page) openSession(botID, sessionID string) {
	p.env.RequestChange(func() {
		if p.settings != nil {
			p.settings.Close()
			p.settings = nil
		}
		p.showConversation(botID, sessionID)
	})
}
func (p *page) openSettings(id, section string) {
	bot := p.bot(id)
	if bot == nil {
		return
	}
	p.env.RequestChange(func() {
		if p.settings != nil {
			p.settings.Close()
		}
		p.settings = newProfileSettings(p, *bot, section)
	})
}
func (p *page) closeContent() {
	p.closeTopic()
	if p.readAloud != nil {
		p.readAloud.stop()
		p.readAloud = nil
	}
	if p.voice != nil {
		p.closeVoice()
	}
	if p.chat != nil {
		p.chat.Close()
		p.chat = nil
	}
	if p.group != nil {
		p.group.Close()
		p.group = nil
	}
	if p.direct != nil {
		p.direct.Close()
		p.direct = nil
	}
	p.chatSession = ""
}
func (p *page) pollEvents() {
	if p.closed || p.polling {
		return
	}
	p.polling = true
	oneBotCall(p, "events", map[string]any{"cursor": p.cursor}, func(result struct {
		Cursor float64 `json:"cursor"`
		Events []struct {
			Type    string          `json:"type"`
			BotID   string          `json:"botId"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
		Reset bool `json:"reset"`
	}, err error) {
		p.polling = false
		delay := time.Second
		if err != nil {
			p.eventError = err.Error()
			delay = 2 * time.Second
		} else {
			p.eventError = ""
			changed := result.Reset
			for _, event := range result.Events {
				if event.Type == "voice" {
					p.receiveVoice(event.Payload)
					continue
				}
				if event.Type == "voice-input" {
					p.receiveVoiceInput(event.Payload)
					continue
				}
				changed = true
				if p.chat != nil {
					var scope struct {
						SessionID string `json:"sessionId"`
					}
					_ = json.Unmarshal(event.Payload, &scope)
					p.chat.BotChanged(event.Type, scope.SessionID, false)
					if p.topic != nil {
						p.topic.BotChanged(event.Type, scope.SessionID, false)
					}
				}
				if p.group != nil && (event.Type == "group-changed" || event.Type == "task-changed") {
					p.group.Refresh(false)
				}
				if p.direct != nil && event.Type == "direct-message-changed" {
					p.direct.Refresh()
				}
				if p.settings != nil {
					p.settings.invalidated(event.Type, event.BotID)
				}
			}
			p.cursor = result.Cursor
			if p.dictation != nil {
				p.dictation.finishFence()
			}
			if changed {
				p.refresh()
			}
		}
		p.cancelPoll = p.after(delay, p.pollEvents)
	})
}
func (p *page) loadPreferences() {
	p.prefs = preferences{Section: "home", Sidebar: 264, LastRead: map[string]int64{}}
	if p.env.Directory == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(p.env.Directory, "desktop-onebot-view.json"))
	if err != nil || len(raw) > 128<<10 {
		return
	}
	var value preferences
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	if value.Sidebar >= 64 && value.Sidebar <= 400 {
		p.prefs.Sidebar = value.Sidebar
	}
	p.prefs.BotID = value.BotID
	p.prefs.ShowReplyTarget = value.ShowReplyTarget
	if value.LastRead != nil {
		p.prefs.LastRead = value.LastRead
	}
}
func (p *page) savePreferences() {
	if p.env.Directory == "" {
		return
	}
	raw, err := json.Marshal(p.prefs)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(p.env.Directory, ".onebot-view-*.tmp")
	if err != nil {
		p.error = err.Error()
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(raw)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(p.env.Directory, "desktop-onebot-view.json"))
	}
	if err != nil {
		p.error = err.Error()
	}
}
func (p *page) sortedProfiles(roster bool) []Profile {
	rows := []Profile{}
	for _, bot := range p.profiles {
		if bot.Status == "deleting" {
			continue
		}
		if !roster && (bot.HiddenAt != nil || bot.Status == "archived") {
			continue
		}
		if p.search != "" && !contains(bot.Name+" "+bot.Description, p.search) {
			continue
		}
		rows = append(rows, bot)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.PinnedAt != nil) != (b.PinnedAt != nil) {
			return a.PinnedAt != nil
		}
		at, bt := int64(0), int64(0)
		if a.LastMessageAt != nil {
			at = *a.LastMessageAt
		}
		if b.LastMessageAt != nil {
			bt = *b.LastMessageAt
		}
		if at != bt {
			return at > bt
		}
		return a.CreatedAt < b.CreatedAt
	})
	return rows
}
func (p *page) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.cancelVoicePause != nil {
		p.cancelVoicePause()
	}
	if p.cancelPoll != nil {
		p.cancelPoll()
	}
	p.closeContent()
	if p.settings != nil {
		p.settings.Close()
	}
	if p.creation != nil && p.creation.portrait != nil {
		p.creation.portrait.Close()
	}
	if p.market.pollCancel != nil {
		p.market.pollCancel()
	}
	if p.importing.pollCancel != nil {
		p.importing.pollCancel()
	}
	if p.dictation != nil {
		p.dictation.Close()
		p.dictation = nil
	}
	p.closeVoice()
	p.savePreferences()
}

// pauseVoiceInput stops every microphone this page holds when the system
// suspends or locks: dictation is cancelled, reading stops, and an open call
// pauses with its draft kept, as Cindy's controller.setBlocked does.
func (p *page) pauseVoiceInput() {
	if p.closed {
		return
	}
	if p.dictation != nil {
		p.dictation.Close()
		p.dictation = nil
	}
	if p.voice != nil {
		p.voice.setBlocked(true)
	}
	if p.readAloud != nil {
		p.readAloud.stop()
	}
}

// View and all modal decisions use the same production native components as
// other WinOne pages, so ui.Tester and real-window checks exercise one path.
func (p *page) View(c *ui.Context) { p.view(c) }

var _ core.Page = (*page)(nil)

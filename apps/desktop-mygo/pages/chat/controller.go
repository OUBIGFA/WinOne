package chat

import (
	"encoding/json"
	"strconv"
	"time"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/widgets"
)

// Chat is the conversation page: the Host's Pi sessions shown natively.
// It follows the React window's ChatPage (features/chat.tsx) request by
// request: the same Host methods in the same order, the same draft and
// queue serialization, the same receipt identities. Heavy tools (editor,
// review, terminal, browser) are the React components in web views.
type Chat struct {
	env *core.Env
	// scope is "onebot.chat." for a OneBot teammate's conversation, whose
	// Host methods live under onebot.chat.
	scope string

	catalog       Catalog
	catalogLoaded bool
	ai            *AIConfiguration
	// providersRaw are the AI services' providers as the Host returned
	// them, for the React tools, which read fields ai does not keep.
	providersRaw json.RawMessage
	// tools are the React tools' web views (panel_host.go).
	tools tools
	// modelsRevision drops a slow AI configuration read that a newer
	// one replaced.
	modelsRevision int

	activeID string
	view     model.State
	text     string
	files    []model.Attachment
	// draftDirty marks a draft not yet saved; saving, one being saved.
	draftDirty, saving bool
	// submitting is the user's message while its send is acknowledged.
	submitting *model.Message

	busy, opening, draftLoading bool
	reconnecting, transferBusy  bool
	err, notice                 string

	activities  map[string]Activity
	activityErr string
	// activityPolling marks the sidebar's chat.activity loop: a read in
	// flight or the next one scheduled, every 1.5 seconds while it shows.
	activityPolling bool

	titleJobs     map[string]titleJob
	titleRequests map[string]bool
	titleEdits    map[string]int

	// draftWrites serializes draft saves across navigation, so a slow
	// earlier save never overwrites a newer draft.
	draftWrites       serial
	draftRevision     int
	openDraftRevision int
	// newDraft and newFiles are the draft of the chat not yet created.
	newDraft string
	newFiles []model.Attachment

	queueWrites  serial
	queueEdit    *queueEdit
	queueWorking map[string]bool
	queuedSend   *queuedSend
	queueOpen    bool

	pendingInput  *receipt
	awaitingInput string
	retryReceipt  *retryReceipt
	sendCleared   bool
	stopping      bool
	stopInFlight  bool
	// sendGeneration cancels an unfinished send's preflight when Stop
	// is pressed.
	sendGeneration int
	lastPrompt     string

	// selection is the conversation chosen last; selectionRevision drops
	// the late results of an earlier choice.
	selection         string
	selectionRevision int
	openings          map[string]*opening
	// silentReload reconnects the event stream of a conversation whose
	// settings changed without replacing its transcript or draft;
	// configuring holds polling while the Host restarts its runtime.
	silentReload, configuring string
	session                   *session
	refreshToken              int

	// The new chat's options, until it is created.
	newProjectID, location, saveLocation string
	newModel                             widgets.AISelection
	newPermission, projectPermission     model.Permission
	newThinking                          string
	newFastMode                          bool

	historyLoading bool
	historyRequest *struct{}

	prefs prefs

	// after schedules fn on the UI thread after d; tests replace it.
	after func(d time.Duration, fn func()) func()

	ui uiState
}

type titleJob struct {
	pending bool
	err     string
}

type receipt struct {
	id           string
	text         string
	attachments  []string
	deliveryMode model.DeliveryMode
}

type retryReceipt struct{ key, id string }

type queueEdit struct {
	id    string
	text  string
	files []model.Attachment
}

type queuedSend struct {
	id    string
	text  string
	files []model.Attachment
}

// opening is a chat.open request shared by every caller asking for the
// same conversation while it runs.
type opening struct {
	done    bool
	result  model.Snapshot
	err     error
	waiters []func(model.Snapshot, error)
}

// session polls one open conversation's events.
type session struct {
	id       string
	cursor   float64
	disposed bool
	cancel   func()
}

// New makes the Chat page.
func New(env *core.Env) *Chat { return newChat(env, "") }

func newChat(env *core.Env, scope string) *Chat {
	p := &Chat{env: env, scope: scope, view: model.Empty(),
		activities: map[string]Activity{}, titleJobs: map[string]titleJob{}, titleRequests: map[string]bool{},
		titleEdits: map[string]int{}, queueWorking: map[string]bool{}, openings: map[string]*opening{},
		newPermission: model.PermissionAsk, projectPermission: model.PermissionAsk}
	p.prefs = loadPrefs(env)
	p.after = func(d time.Duration, fn func()) func() {
		t := time.AfterFunc(d, func() { env.Post(fn) })
		return func() { t.Stop() }
	}
	p.ui.sidebar = true
	p.loadCatalog()
	return p
}

// method is a Chat method's Host name in the page's scope.
func (p *Chat) method(name string) string { return "chat." + p.scope + name }

// chat sends a Chat method and decodes its result on the UI thread.
func chatCall[T any](p *Chat, name string, params any, done func(T, error)) {
	call(p.env, p.method(name), params, done)
}

// run runs one user action at a time, as the React page's run: it
// clears the last error and notice, marks the page busy, and shows the
// action's error.
func (p *Chat) run(action func(done func(error))) {
	if p.busy {
		return
	}
	p.busy, p.err, p.notice = true, "", ""
	action(func(err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
		}
	})
}

// loadCatalog reads the catalog and the AI services the first time.
func (p *Chat) loadCatalog() {
	p.refresh(func(err error) {
		if err != nil {
			p.err = err.Error()
		}
	})
}

// refresh reads the catalog and the AI services again; done runs after
// both or the first failure. The models apply when no newer read began.
func (p *Chat) refresh(done func(error)) {
	p.modelsRevision++
	revision := p.modelsRevision
	var catalog Catalog
	var models AIConfiguration
	var providers json.RawMessage
	core.Do(p.env, func() error {
		if err := p.env.Host.CallInto(p.method("catalog"), map[string]any{}, &catalog); err != nil {
			return err
		}
		raw, err := p.env.Host.Call("capture.ai.configuration.get", nil)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &models); err != nil {
			return err
		}
		var configuration struct {
			Providers json.RawMessage `json:"providers"`
		}
		if json.Unmarshal(raw, &configuration) == nil {
			providers = configuration.Providers
		}
		return nil
	}, func(err error) {
		if err != nil {
			done(err)
			return
		}
		p.catalog, p.catalogLoaded = catalog, true
		if revision == p.modelsRevision {
			p.ai, p.providersRaw = &models, providers
			p.newModel = p.initialModel(&models, p.newModel)
		}
		done(nil)
	})
}

// initialModel is the new chat's model: the last one chosen while it is
// still enabled, else the chat task's, the default task's, or the first
// enabled model.
func (p *Chat) initialModel(models *AIConfiguration, current widgets.AISelection) widgets.AISelection {
	if last := p.prefs.LastModel; last != nil {
		for _, provider := range models.Providers {
			if provider.ID == last.Provider {
				for _, m := range provider.Models {
					if m == last.Model {
						return *last
					}
				}
			}
		}
	}
	for _, task := range []string{"chat", "default"} {
		if s := models.Tasks[task]; s != nil {
			return *s
		}
	}
	for _, provider := range models.Providers {
		if len(provider.Models) > 0 {
			return widgets.AISelection{Provider: provider.ID, Model: provider.Models[0]}
		}
	}
	return current
}

// pollActivity reads every conversation's activity for the sidebar, at
// most one read at a time, every 1.5 seconds while the sidebar shows.
func (p *Chat) pollActivity() {
	if p.activityPolling || p.closed() {
		return
	}
	p.activityPolling = true
	p.readActivity()
}

func (p *Chat) readActivity() {
	if p.closed() || !p.ui.sidebar {
		p.activityPolling = false
		return
	}
	chatCall(p, "activity", map[string]any{}, func(value map[string]Activity, err error) {
		if err != nil {
			p.activityErr = err.Error()
		} else {
			if value == nil {
				value = map[string]Activity{}
			}
			p.activities, p.activityErr = value, ""
		}
		p.after(1500*time.Millisecond, p.readActivity)
	})
}

func (p *Chat) closed() bool { return p.ui.closed }

// conversation is the open conversation's record, nil for a new chat.
func (p *Chat) conversation() *Conversation { return p.catalog.conversation(p.activeID) }

// sidebarActivities is the activity of every conversation, the open
// one's from its live state.
func (p *Chat) sidebarActivity(id string) Activity {
	if id == p.activeID && !p.opening {
		v := p.view
		phase := "idle"
		switch {
		case v.Status == "stopped":
			phase = "interrupted"
		case v.Status == "failed":
			phase = "error"
		case len(v.Approvals) > 0:
			phase = "needs-interaction"
		case v.Running || v.BackgroundRunning:
			phase = "running"
		case v.Status == "completed":
			phase = "completed"
		}
		return Activity{Phase: phase, Attention: v.Status == "failed", Paused: v.Paused != nil && *v.Paused}
	}
	return p.activities[id]
}

// updateText changes the composer's draft and saves it, serialized behind
// earlier saves; the chat not yet created keeps it locally.
func (p *Chat) updateText(value string, files []model.Attachment, id string) {
	p.text, p.files = value, files
	if id == "" {
		p.newDraft, p.newFiles = value, files
		p.draftDirty = value != "" || len(files) > 0
		return
	}
	if p.queueEdit != nil {
		p.draftDirty = true
		return
	}
	p.draftRevision++
	revision := p.draftRevision
	p.draftDirty, p.saving = true, true
	draft := map[string]any{"text": value, "attachments": nonNil(files)}
	p.draftWrites.run(func(done func()) {
		chatCall(p, "draft.save", map[string]any{"id": id, "draft": draft}, func(_ json.RawMessage, err error) {
			if revision == p.draftRevision {
				p.saving = false
				if err != nil {
					p.err = i18n.T("chat.saveFailed") + ": " + err.Error()
				} else {
					p.draftDirty = false
				}
			}
			done()
		})
	})
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// choose opens another conversation ("" is a new chat) once earlier
// draft saves finished. A repeated choice does nothing; a newer choice
// drops this one.
func (p *Chat) choose(id string, stillRequested func() bool, done func(error)) {
	if id == p.selection && id != "" {
		done(nil)
		return
	}
	if p.queueEdit != nil {
		done(core.Errorf(i18n.T("chat.queue_finishEdit")))
		return
	}
	if p.ui.filesDirty {
		done(core.Errorf(i18n.T("chat.saveFileBeforeNavigate")))
		return
	}
	p.selectionRevision++
	revision := p.selectionRevision
	p.draftWrites.after(func() {
		if revision != p.selectionRevision || (stillRequested != nil && !stillRequested()) {
			done(nil)
			return
		}
		if p.ui.filesDirty {
			done(core.Errorf(i18n.T("chat.saveFileBeforeNavigate")))
			return
		}
		p.show(id)
		done(nil)
	})
}

// show makes id the open conversation, with fresh runtime state.
func (p *Chat) show(id string) {
	previous := p.activeID
	p.ui.details = false
	p.historyRequest, p.historyLoading = nil, false
	p.configuring, p.silentReload, p.reconnecting = "", "", false
	p.selection = id
	p.draftRevision++
	p.openDraftRevision = p.draftRevision
	p.pendingInput, p.awaitingInput, p.retryReceipt, p.queuedSend = nil, "", nil, nil
	p.ui.resetDetails()
	p.err = ""
	p.activeID = id
	p.opening, p.draftLoading = id != "", id != ""
	p.draftDirty = id == "" && (p.newDraft != "" || len(p.newFiles) > 0)
	if id == "" {
		p.text, p.files = p.newDraft, p.newFiles
	} else {
		p.text, p.files = "", nil
	}
	p.view = model.Empty()
	p.notice = ""
	if previous != id {
		p.release(previous)
		p.open(false)
	}
}

// release lets the Host stop the runtime of a conversation the page no
// longer shows, after its queued edits finished.
func (p *Chat) release(id string) {
	if p.session != nil {
		p.session.disposed = true
		if p.session.cancel != nil {
			p.session.cancel()
		}
		p.session = nil
	}
	if id == "" {
		return
	}
	spawn(p.env, func() { _, _ = p.env.Host.Call(p.method("release"), map[string]any{"id": id}) })
}

func spawn(env *core.Env, fn func()) {
	if env.Spawn != nil {
		env.Spawn(fn)
		return
	}
	go fn()
}

// openConversation shares one chat.open of id among its callers.
func (p *Chat) openConversation(id string, done func(model.Snapshot, error)) {
	o := p.openings[id]
	if o == nil {
		o = &opening{}
		p.openings[id] = o
		chatCall(p, "open", map[string]any{"id": id}, func(s model.Snapshot, err error) {
			o.done, o.result, o.err = true, s, err
			delete(p.openings, id)
			for _, waiter := range o.waiters {
				waiter(s, err)
			}
			o.waiters = nil
		})
	}
	if o.done {
		done(o.result, o.err)
		return
	}
	o.waiters = append(o.waiters, done)
}

// open loads the active conversation: its draft and its snapshot, then
// follows its events. silent reconnects the events of a conversation
// whose settings changed without replacing its transcript or draft.
func (p *Chat) open(silent bool) {
	id := p.activeID
	if id == "" {
		return
	}
	s := &session{id: id}
	if p.session != nil {
		p.session.disposed = true
		if p.session.cancel != nil {
			p.session.cancel()
		}
	}
	p.session = s
	loadRevision := p.openDraftRevision
	if !silent {
		p.opening, p.draftLoading = true, true
		chatCall(p, "draft.get", map[string]any{"id": id}, func(d struct {
			Text            string             `json:"text"`
			PendingID       string             `json:"pendingId"`
			PendingDelivery model.DeliveryMode `json:"pendingDelivery"`
			Attachments     []model.Attachment `json:"attachments"`
		}, err error) {
			if s.disposed {
				return
			}
			p.draftLoading = false
			if err != nil {
				p.err = i18n.T("chat.draftLoadFailed") + ": " + err.Error()
				return
			}
			if loadRevision == p.draftRevision {
				if d.PendingID != "" {
					mode := d.PendingDelivery
					if mode == "" {
						mode = model.DeliveryQueue
					}
					var paths []string
					for _, f := range d.Attachments {
						paths = append(paths, f.Path)
					}
					p.pendingInput = &receipt{id: d.PendingID, text: d.Text, attachments: paths, deliveryMode: mode}
				}
				p.text, p.files, p.draftDirty = d.Text, d.Attachments, false
			}
		})
	}
	p.openConversation(id, func(state model.Snapshot, err error) {
		if s.disposed {
			return
		}
		p.configuring, p.reconnecting = "", false
		if err != nil {
			p.opening = false
			p.err = err.Error()
			return
		}
		if silent {
			p.view.ModelError = state.ModelError
		} else {
			p.view = model.FromSnapshot(state)
			p.view.Running = state.Streaming
			p.opening = false
			p.ui.followLatest()
		}
		p.poll(s)
	})
}

// poll reads the session's next events and schedules the next read: at
// once while more wait, every 160 ms while it runs, else every 1.2 s.
func (p *Chat) poll(s *session) {
	if s.disposed {
		return
	}
	if p.configuring == s.id {
		s.cancel = p.after(160*time.Millisecond, func() { p.poll(s) })
		return
	}
	chatCall(p, "poll", map[string]any{"id": s.id, "cursor": s.cursor}, func(page model.Poll, err error) {
		if s.disposed {
			return
		}
		if err != nil {
			if p.configuring != s.id {
				p.err = err.Error()
			}
			s.cancel = p.after(2*time.Second, func() { p.poll(s) })
			return
		}
		waiting := p.awaitingInput
		if waiting != "" {
			for _, e := range page.Events {
				d := e.Data
				if d.Type == "input_saved" && d.ID == waiting || d.Type == "settled" && d.RequestID == waiting || d.State != nil && hasRequest(d.State.Messages, waiting) {
					p.awaitingInput = ""
					break
				}
			}
		}
		if page.Status == "stopped" || page.Status == "paused" {
			p.awaitingInput = ""
		}
		next := func() {
			s.cursor = page.Cursor
			if page.Error != "" {
				p.err = page.Error
			}
			delay := 1200 * time.Millisecond
			if page.More {
				delay = 0
			} else if page.Running {
				delay = 160 * time.Millisecond
			}
			s.cancel = p.after(delay, func() { p.poll(s) })
		}
		if page.Reset {
			chatCall(p, "state", map[string]any{"id": s.id}, func(state model.Snapshot, err error) {
				if s.disposed {
					return
				}
				if err != nil {
					p.err = err.Error()
					s.cancel = p.after(2*time.Second, func() { p.poll(s) })
					return
				}
				if p.awaitingInput != "" && hasRequest(state.Messages, p.awaitingInput) {
					p.awaitingInput = ""
				}
				p.view = model.FromSnapshot(state)
				p.view.Running, p.view.Status, p.view.Approvals = page.Running || p.awaitingInput != "", page.Status, page.Approvals
				p.ui.contentChanged()
				next()
			})
			return
		}
		for _, e := range page.Events {
			p.view = model.Apply(p.view, e.Data)
		}
		p.view.Running, p.view.Status, p.view.Approvals = page.Running || p.awaitingInput != "", page.Status, page.Approvals
		// Compaction feedback and the completion chime belong to the
		// transcript once, not to the event reducer.
		for _, e := range page.Events {
			d := e.Data
			if d.Type == "compacted" && d.Before != nil && d.After != nil {
				p.notice = i18n.T("chat.compacted", "before", number(*d.Before), "after", number(*d.After))
			}
			if d.Type == "settled" && d.Status == "completed" && p.prefs.Sound {
				playDone()
			}
		}
		if len(page.Events) > 0 {
			p.ui.contentChanged()
		}
		next()
	})
}

func hasRequest(messages []model.Message, request string) bool {
	for _, m := range messages {
		if m.RequestID == request {
			return true
		}
	}
	return false
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// reconnect follows the active conversation's events again after its
// settings changed, keeping its transcript and draft.
func (p *Chat) reconnect() {
	p.silentReload = p.activeID
	p.open(true)
}

// loadEarlier reads the page of history before the oldest loaded.
func (p *Chat) loadEarlier() {
	if p.historyRequest != nil || p.activeID == "" || p.opening || !p.view.HasOlder || p.view.Before == "" {
		return
	}
	id, revision, request := p.activeID, p.selectionRevision, &struct{}{}
	p.historyRequest, p.historyLoading = request, true
	chatCall(p, "state", map[string]any{"id": id, "before": p.view.Before}, func(page model.Snapshot, err error) {
		current := p.selection == id && p.selectionRevision == revision && p.historyRequest == request
		if p.historyRequest == request {
			p.historyRequest, p.historyLoading = nil, false
		}
		if !current {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.ui.keepPosition()
		ids := map[string]bool{}
		for _, m := range p.view.Messages {
			if m.ID != "" {
				ids[m.ID] = true
			}
		}
		var older []model.Message
		for _, m := range page.Messages {
			if m.ID == "" || !ids[m.ID] {
				older = append(older, m)
			}
		}
		p.view.HasOlder, p.view.Before = page.HasOlder, page.Before
		p.view.Messages = append(older, p.view.Messages...)
		p.view.Operations = model.MergeOperations(page.Operations, p.view.Operations)
	})
}

// Close leaves the page: the open conversation is released and the
// queue's locks reset, as the React page did when it unmounted.
func (p *Chat) Close() {
	if p.ui.closed {
		return
	}
	p.ui.closed = true
	if id := p.selection; id != "" {
		p.queueWrites.after(func() {
			spawn(p.env, func() { _, _ = p.env.Host.Call(p.method("queue"), map[string]any{"id": id, "action": "resetLocks"}) })
		})
	}
	p.release(p.activeID)
	p.closeTools()
	p.ui.close()
}

// Package pluginhost is the window's side of the local plugin platform: the
// confirmations, pickers and notices a plugin's slots ask of the user, and the
// native rendering of a plugin's cards. Every rule stays in the plugin host; this
// package shows what it asks and returns exactly what the user answered.
package pluginhost

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/pluginchecks"
	"winone/desktopmygo/chat/pluginhost/globalpages"
	"winone/desktopmygo/chat/pluginhost/mcpapp"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/widgets"
)

const (
	activePoll = 700 * time.Millisecond
	idlePoll   = 4 * time.Second
)

func text(key string, args ...any) string { return i18n.T("pluginHost."+key, args...) }

type event struct {
	Sequence uint64          `json:"sequence"`
	Channel  string          `json:"channel"`
	Payload  json.RawMessage `json:"payload"`
}

type request struct {
	Kind        string                    `json:"kind"`
	GhostName   string                    `json:"ghostName"`
	Body        string                    `json:"body"`
	ConfirmText string                    `json:"confirmText"`
	CancelText  string                    `json:"cancelText"`
	Danger      bool                      `json:"danger"`
	Purpose     string                    `json:"purpose"`
	Multiple    bool                      `json:"multiple"`
	Accept      []string                  `json:"accept"`
	DefaultPath string                    `json:"defaultPath"`
	Key         string                    `json:"key"`
	Label       string                    `json:"label"`
	Path        string                    `json:"path"`
	Lane        string                    `json:"lane"`
	Items       []grantItem               `json:"items"`
	Request     json.RawMessage           `json:"request"`
	PluginID    string                    `json:"pluginId"`
	Version     string                    `json:"version"`
	Permissions []pluginchecks.Permission `json:"permissions"`
}

type grantItem struct {
	Name        string `json:"name"`
	AbsPath     string `json:"absPath"`
	IsDirectory bool   `json:"isDirectory"`
}

type prompt struct {
	ID      string          `json:"id"`
	Request json.RawMessage `json:"request"`
	parsed  request
	started bool
	value   string
	loadErr string
	model   widgets.AISelection
	effort  string
	fast    bool
	// installing keeps the install dialog open while the package is written.
	installing bool
	setup      *mcpapp.SetupPrompt
	submitting bool
}

// Host polls for what plugins ask, on one goroutine for the window's life.
type Host struct {
	env      *core.Env
	cursor   uint64
	prompts  []*prompt
	seen     map[string]bool
	cards    *cards
	once     sync.Once
	stop     chan struct{}
	closed   bool
	mailFast atomic.Bool
	pollWake chan struct{}
	manager  *pluginchecks.Manager
	// Observers of the push channels pages care about, called on the UI thread.
	OnPreview func(payload json.RawMessage)
	// Unread marks plugins that asked for attention with a badge.
	Unread map[string]bool
	// Native team mutations invalidate catalog metadata without navigating
	// away from the owner's current task or disturbing its draft.
	TeamRevision              uint64
	replies                   map[string]map[string]*replyOverride
	repliesAsked              map[string]bool
	pages                     *globalpages.Controller
	previewRequests           map[string][]json.RawMessage
	taskViews                 map[string][]string
	focusedSession, focusSent string
	focusSending              bool
}

var (
	hostsMu sync.Mutex
	hosts   = map[*core.Env]*Host{}
)

// For is the window's plugin host.
func For(env *core.Env) *Host {
	hostsMu.Lock()
	defer hostsMu.Unlock()
	h := hosts[env]
	if h == nil {
		h = &Host{env: env, stop: make(chan struct{}), pollWake: make(chan struct{}, 1), seen: map[string]bool{}, cards: newCards(env), Unread: map[string]bool{}, replies: map[string]map[string]*replyOverride{}, repliesAsked: map[string]bool{}, pages: globalpages.For(env), previewRequests: map[string][]json.RawMessage{}}
		hosts[env] = h
	}
	return h
}

// Close releases window-owned pages and polling; it never stops the Host's
// scheduled work. Late asynchronous callbacks must not revive a closed view.
func (h *Host) Close() {
	if h.closed {
		return
	}
	h.closed = true
	h.MailClose()
	for _, pane := range h.cards.media {
		pane.Close()
	}
	clear(h.cards.media)
	close(h.stop)
	h.pages.Close()
	if h.manager != nil {
		h.manager.Close()
		h.manager = nil
	}
	for _, p := range h.prompts {
		if p.setup != nil {
			p.setup.Clear()
		}
	}
	h.previewRequests = nil
	h.prompts = nil
	clear(h.seen)
	hostsMu.Lock()
	if hosts[h.env] == h {
		delete(hosts, h.env)
	}
	hostsMu.Unlock()
}

// FlushAllMailEdits protects root navigation/window teardown, including mail
// owned by a kept OneBot topic/controller. Each delivery uses the mail worker's
// original session barrier; this does not clear validation or provider drafts.
func (h *Host) FlushAllMailEdits(done func(bool)) {
	sessions := map[string]bool{}
	for _, item := range h.cards.byCall {
		if item.Mail != nil && mailCanEdit(item) && item.SessionID != "" {
			sessions[item.SessionID] = true
		}
	}
	if len(sessions) == 0 {
		done(true)
		return
	}
	remaining, successful := len(sessions), true
	for session := range sessions {
		h.FlushMailEdits(session, func(ok bool) {
			successful = successful && ok
			remaining--
			if remaining == 0 {
				if successful && h.env.Activity != nil {
					for _, item := range h.cards.byCall {
						if item.Mail != nil && !item.formDirty && !item.formSending && !item.busy {
							h.env.Activity.Report("plugin-mail:"+item.CallID, false, false, nil)
						}
					}
				}
				done(successful)
			}
		})
	}
}

// QueueMailBrowser retains the exact reader session when another conversation
// controller was bound last. The receiver bypasses only the mail probe, never
// the mail edit/draft delivery barrier or ordinary native browser navigation.
func (h *Host) QueueMailBrowser(session, address string) {
	if h.closed || session == "" || safeMailLink(address) == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{"sessionId": session, "url": address, "mailReaderBypass": true})
	h.previewRequests[session] = append(h.previewRequests[session], payload)
	if h.env.Post != nil {
		h.env.Post(func() {})
	}
}

func (h *Host) poll() {
	var after uint64 // Owned by this goroutine, not the UI's cursor field.
	for {
		select {
		case <-h.stop:
			return
		default:
		}
		var page struct {
			Cursor  uint64    `json:"cursor"`
			Events  []event   `json:"events"`
			Prompts []*prompt `json:"prompts"`
		}
		err := h.env.Host.CallInto("chat.ghosts.pending", map[string]uint64{"after": after}, &page)
		wait := idlePoll
		if err == nil {
			after = page.Cursor
			h.env.Post(func() { h.apply(page.Cursor, page.Events, page.Prompts) })
			if len(page.Events) > 0 || len(page.Prompts) > 0 {
				wait = activePoll
			}
		}
		if h.mailFast.Load() {
			wait = 250 * time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-h.pollWake:
			timer.Stop()
		case <-h.stop:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *Host) apply(cursor uint64, events []event, prompts []*prompt) {
	if h.closed {
		return
	}
	live := make(map[string]bool, len(prompts))
	for _, p := range prompts {
		live[p.ID] = true
	}
	retained := h.prompts[:0]
	for _, p := range h.prompts {
		if live[p.ID] {
			retained = append(retained, p)
		} else if p.setup != nil {
			p.setup.Clear()
		}
	}
	h.prompts = retained
	for id := range h.seen {
		if !live[id] {
			delete(h.seen, id)
		}
	}
	previousCursor := h.cursor
	h.cursor = max(h.cursor, cursor)
	for _, e := range events {
		// Prompt snapshots may change at the same cursor, but an old push
		// must never replay mail actions/notice/reveal after the next poll.
		if e.Sequence > 0 && e.Sequence <= previousCursor {
			continue
		}
		h.handle(e)
	}
	for _, next := range prompts {
		if h.seen[next.ID] {
			continue
		}
		h.seen[next.ID] = true
		if json.Unmarshal(next.Request, &next.parsed) != nil {
			h.answer(next, map[string]any{})
			continue
		}
		h.prompts = append(h.prompts, next)
	}
}

func (h *Host) handle(e event) {
	switch e.Channel {
	case "changed":
		h.pages.Refresh()
	case "plugin-setup":
		var update struct {
			Request json.RawMessage `json:"request"`
		}
		if json.Unmarshal(e.Payload, &update) == nil && len(update.Request) > 0 {
			var next struct {
				RequestID string `json:"requestId"`
			}
			_ = json.Unmarshal(update.Request, &next)
			for _, p := range h.prompts {
				var current struct {
					RequestID string `json:"requestId"`
				}
				_ = json.Unmarshal(p.parsed.Request, &current)
				if p.setup != nil && next.RequestID != "" && next.RequestID == current.RequestID {
					if err := p.setup.Update(update.Request); err == nil {
						p.parsed.Request = update.Request
					}
				}
			}
		}
	case "preview-media":
		h.pages.PreviewMedia(e.Payload)
	case "team-changed":
		h.TeamRevision++
	case "notify":
		var notice struct {
			Text, GhostName string
		}
		if json.Unmarshal(e.Payload, &notice) == nil && notice.Text != "" && h.env.Toast != nil {
			h.env.Toast(notice.Text)
		}
	case "card":
		h.cards.update(e.Payload)
	case "reveal-session":
		var target struct {
			SessionID string `json:"sessionId"`
			BotID     string `json:"botId"`
		}
		if json.Unmarshal(e.Payload, &target) == nil && target.SessionID != "" && h.env.Open != nil {
			if target.BotID != "" {
				options, _ := json.Marshal(map[string]string{"botId": target.BotID, "sessionId": target.SessionID})
				h.env.Open("onebot", core.Intent{Requested: true, Options: options})
			} else {
				h.env.Open("chat", core.Intent{Requested: true, Conversation: target.SessionID})
			}
		}
	case "show-item":
		var target struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(e.Payload, &target) == nil && target.Path != "" {
			go mygo.Shell.ShowItemInFolder(target.Path)
		}
	case "clipboard-png":
		var image struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(e.Payload, &image) == nil {
			if data, err := base64.StdEncoding.DecodeString(image.Data); err == nil {
				_ = mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
			}
		}
	case "preview":
		var target struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(e.Payload, &target) == nil && target.SessionID != "" {
			h.previewRequests[target.SessionID] = append(h.previewRequests[target.SessionID], append(json.RawMessage(nil), e.Payload...))
		} else if h.OnPreview != nil {
			h.OnPreview(e.Payload)
		}
	case "task-view":
		var view struct {
			InstanceID string `json:"instanceId"`
			SessionID  string `json:"sessionId"`
		}
		if json.Unmarshal(e.Payload, &view) == nil && view.InstanceID != "" && view.SessionID != "" {
			if h.taskViews == nil {
				h.taskViews = map[string][]string{}
			}
			for _, id := range h.taskViews[view.SessionID] {
				if id == view.InstanceID {
					return
				}
			}
			h.taskViews[view.SessionID] = append(h.taskViews[view.SessionID], view.InstanceID)
		}
	case "badge":
		var badge struct {
			GhostID string `json:"ghostId"`
			Unread  bool   `json:"unread"`
			Summary string `json:"summary"`
			At      int64  `json:"at"`
		}
		if json.Unmarshal(e.Payload, &badge) == nil {
			if badge.Unread {
				h.Unread[badge.GhostID] = true
				h.pages.Badge(badge.GhostID, &globalpages.UnreadEntry{GhostID: badge.GhostID, Summary: badge.Summary, At: badge.At})
			} else {
				delete(h.Unread, badge.GhostID)
				h.pages.Badge(badge.GhostID, nil)
			}
		}
	case "assistant-rewritten":
		var value replyOverride
		if json.Unmarshal(e.Payload, &value) == nil {
			h.storeRewrite(value)
		}
	case "schedule-draft":
		if json.Valid(e.Payload) && h.env.Open != nil {
			h.env.Open("chat", core.Intent{Requested: true, AutomationDraft: append(json.RawMessage(nil), e.Payload...)})
		}
	}
}

func (h *Host) answer(p *prompt, value any) {
	for index, next := range h.prompts {
		if next == p {
			h.prompts = append(h.prompts[:index], h.prompts[index+1:]...)
			break
		}
	}
	id := p.ID
	core.Do(h.env, func() error {
		return h.env.Host.CallInto("chat.ghosts.answer", map[string]any{"id": id, "answer": value}, nil)
	}, func(err error) {
		if err != nil && h.env.Toast != nil {
			h.env.Toast(err.Error())
		}
	})
}

// View shows the oldest open request. It belongs at the window's root, so a plugin
// can ask while any page is open.
func (h *Host) View(c *ui.Context) {
	if h.closed {
		return
	}
	for source, pane := range h.cards.media {
		pane.SetVisible(h.cards.mediaVisible[source])
	}
	clear(h.cards.mediaVisible)
	h.once.Do(func() { go h.poll() })
	h.pages.MediaDialogs(c)
	h.MailView(c)
	fast := false
	for _, item := range h.cards.byCall {
		fast = fast || item.Mail != nil && (item.Mail.PendingSend != nil || item.busy || item.formSending)
	}
	if !h.mailFast.Swap(fast) && fast {
		select {
		case h.pollWake <- struct{}{}:
		default:
		}
	}
	if h.manager != nil {
		h.manager.View(c)
	}
	if len(h.prompts) == 0 {
		return
	}
	p := h.prompts[0]
	switch p.parsed.Kind {
	case "clipboard-file", "clipboard-png":
		if !p.started {
			p.started = true
			var data transfer.Data
			var err error
			if p.parsed.Kind == "clipboard-file" {
				data, err = transfer.FileData(p.parsed.Path)
			} else {
				var bytes []byte
				bytes, err = os.ReadFile(p.parsed.Path)
				if err == nil {
					data = transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, bytes)))
				}
			}
			if err == nil {
				err = mygo.Clipboard.Write(data)
			}
			if err != nil {
				h.answer(p, map[string]any{"copied": false, "message": err.Error()})
			} else {
				h.answer(p, map[string]any{"copied": true})
			}
		}
	case "plugin-setup":
		h.setupDialog(c, p)
	case "plugin-settings", "plugin-client-settings", "open-connection-card":
		h.settingsRequest(c, p)
	case "mcp-ui-approval":
		h.mcpApproval(c, p)
	case "confirm":
		h.confirmDialog(c, p, p.parsed.Body, false)
	case "grant":
		body := text("grant", "name", p.parsed.GhostName, "lane", p.parsed.Lane)
		for _, item := range p.parsed.Items {
			body += "\n" + first(item.AbsPath, item.Name)
		}
		h.confirmDialog(c, p, body, true)
	case "pick-directory", "pick-files", "save":
		h.nativePicker(p)
	case "secret":
		h.secretDialog(c, p)
	case "model-picker":
		h.modelDialog(c, p)
	case "install":
		h.installDialog(c, p)
	default:
		h.answer(p, map[string]any{"ok": false, "reason": "UNSUPPORTED_NATIVE_REQUEST"})
	}
}

func (h *Host) setupDialog(c *ui.Context, p *prompt) {
	if p.setup == nil {
		var err error
		p.setup, err = mcpapp.NewSetupPrompt(p.parsed.Request)
		if err != nil {
			p.loadErr = err.Error()
		}
	}
	open := true
	kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: first(p.parsed.GhostName, text("title")), Width: 560, Cancel: text("cancel"), Busy: p.submitting, Error: p.loadErr, Body: func() {
		if p.setup == nil {
			return
		}
		p.setup.OnOpenExternal = func(address string) {
			if safeNativeExternal(address) {
				c.Services().OpenURL(address)
			}
		}
		if answer, clicked := p.setup.View(c); clicked {
			if answer["action"] == "submit" {
				h.submitConnection(p, answer)
			} else {
				h.answer(p, answer)
			}
		}
	}})
	if !open {
		if p.setup != nil {
			p.setup.Clear()
		}
		h.answer(p, map[string]any{"action": "cancel"})
	}
}

func safeNativeExternal(address string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && ui.SafeToOpen(address)
}
func (h *Host) currentPrompt(p *prompt) bool {
	for _, next := range h.prompts {
		if next == p {
			return true
		}
	}
	return false
}

func (h *Host) submitConnection(p *prompt, answer map[string]any) {
	if p.submitting {
		return
	}
	p.submitting = true
	core.Do(h.env, func() error {
		return h.env.Host.CallInto("chat.ghosts.connection-submit", map[string]any{"id": p.ID, "answer": answer}, nil)
	}, func(err error) {
		p.submitting = false
		if h.closed || !h.currentPrompt(p) {
			return
		}
		if err != nil {
			p.loadErr = err.Error()
			return
		}
		for i, next := range h.prompts {
			if next == p {
				h.prompts = append(h.prompts[:i], h.prompts[i+1:]...)
				break
			}
		}
		if p.setup != nil {
			p.setup.Clear()
		}
	})
}
func (h *Host) settingsRequest(c *ui.Context, p *prompt) {
	plugin := p.parsed.PluginID
	if plugin != "" && !p.started {
		p.started = true
		h.env.RequestChange(func() {
			if h.closed || !h.currentPrompt(p) {
				return
			}
			if h.manager != nil {
				h.manager.Close()
			}
			h.manager = pluginchecks.NewManager(h.env)
			h.manager.OpenTarget(plugin, p.parsed.Kind == "plugin-settings")
		})
	}
	if h.manager != nil && h.manager.TargetOpened(plugin) {
		h.answer(p, map[string]any{"ok": true})
		return
	}
	open := true
	message := text("configure")
	if h.manager != nil && h.manager.Error() != "" {
		message = h.manager.Error()
	}
	kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: first(p.parsed.GhostName, text("title")), Message: message, Cancel: text("cancel"), Width: 460})
	if !open {
		h.answer(p, map[string]any{"ok": false, "reason": "cancelled"})
	}
}
func (h *Host) mcpApproval(c *ui.Context, p *prompt) {
	open := true
	permissions := p.parsed.Permissions
	if len(permissions) == 0 && len(p.parsed.Request) > 0 {
		var details struct {
			Permissions []pluginchecks.Permission `json:"permissions"`
		}
		if json.Unmarshal(p.parsed.Request, &details) == nil {
			permissions = details.Permissions
		}
	}
	if kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: i18n.T("pluginViews.interactiveApproval", "name", p.parsed.GhostName, "version", p.parsed.Version), Width: 640, Confirm: text("allow"), Cancel: text("cancel"), ConfirmDisabled: !pluginchecks.PermissionsKnown(permissions), Body: func() { pluginchecks.PermissionDetails(c, permissions) }}) {
		h.answer(p, map[string]any{"confirmed": true})
	} else if !open {
		h.answer(p, map[string]any{"confirmed": false})
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (h *Host) confirmDialog(c *ui.Context, p *prompt, body string, trusted bool) {
	t := c.Theme()
	open := true
	title := first(p.parsed.GhostName, text("title"))
	if kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: title, Width: 460,
		Confirm: first(p.parsed.ConfirmText, text("allow")), Danger: p.parsed.Danger, Cancel: first(p.parsed.CancelText, text("deny")),
		Body: func() {
			ui.Text(c, body).Selectable().MinWidth(0)
			if !trusted {
				ui.Text(c, text("from", "name", title)).TextColor(t.TextMuted).FontSize(kit.HelpSize(c))
			}
		}}) {
		h.answer(p, map[string]any{"confirmed": true})
	} else if !open {
		h.answer(p, map[string]any{"confirmed": false, "reason": "cancelled"})
	}
}

func extensionsOf(accept []string) []string {
	out := []string{}
	for _, value := range accept {
		switch {
		case len(value) > 1 && value[0] == '.':
			out = append(out, value[1:])
		case value == "image/*":
			out = append(out, "png", "jpg", "jpeg", "gif", "webp", "avif")
		case value == "video/*":
			out = append(out, "mp4", "webm", "mov")
		case value == "audio/*":
			out = append(out, "mp3", "wav", "ogg", "m4a")
		case value == "application/pdf":
			out = append(out, "pdf")
		case value == "text/plain":
			out = append(out, "txt")
		case value == "application/zip":
			out = append(out, "zip")
		}
	}
	return out
}

// nativePicker runs the system's dialog off the UI thread, once for the prompt.
func (h *Host) nativePicker(p *prompt) {
	if p.started {
		return
	}
	p.started = true
	if h.env.Dialogs == nil {
		h.answer(p, map[string]any{})
		return
	}
	r := p.parsed
	title := first(r.Purpose, text("pickTitle", "name", r.GhostName))
	core.Go(h.env, func() (map[string]any, error) {
		switch r.Kind {
		case "pick-directory":
			path, err := h.env.Dialogs.PickFolder(title)
			return map[string]any{"path": path}, err
		case "save":
			path, err := h.env.Dialogs.SaveFile(text("saveTitle", "name", r.GhostName), core.Filter{}, r.DefaultPath)
			return map[string]any{"path": path}, err
		}
		filter := core.Filter{}
		if extensions := extensionsOf(r.Accept); len(extensions) > 0 {
			filter = core.Filter{Name: r.GhostName, Extensions: extensions}
		}
		if r.Multiple {
			paths, err := h.env.Dialogs.PickFiles(title, filter)
			return map[string]any{"paths": paths}, err
		}
		path, err := h.env.Dialogs.PickFile(title, filter)
		if path == "" {
			return map[string]any{"paths": []string{}}, err
		}
		return map[string]any{"paths": []string{path}}, err
	}, func(value map[string]any, err error) {
		if err != nil && h.env.Toast != nil {
			h.env.Toast(err.Error())
		}
		h.answer(p, value)
	})
}

func (h *Host) secretDialog(c *ui.Context, p *prompt) {
	open := true
	if kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: first(p.parsed.GhostName, text("title")), Width: 460, Confirm: text("save"), Cancel: text("cancel"), ConfirmDisabled: p.value == "",
		Body: func() {
			ui.Text(c, text("secret", "label", first(p.parsed.Label, p.parsed.Key))).MinWidth(0)
			kit.TextInput(c, &p.value).Password().FillWidth().Label(first(p.parsed.Label, p.parsed.Key))
		}}) {
		h.answer(p, map[string]any{"value": p.value})
	} else if !open {
		h.answer(p, map[string]any{})
	}
}

func (h *Host) modelDialog(c *ui.Context, p *prompt) {
	var picker struct {
		RequestID string `json:"requestId"`
		Initial   struct {
			ProviderID, Model, Effort string
			FastMode                  bool
		} `json:"initial"`
	}
	_ = json.Unmarshal(p.parsed.Request, &picker)
	type row struct {
		ID, ProviderID, ProviderName, Name string
		Efforts                            []string
	}
	if !p.started {
		p.started = true
		p.model = widgets.AISelection{Provider: picker.Initial.ProviderID, Model: picker.Initial.Model}
		p.effort, p.fast = picker.Initial.Effort, picker.Initial.FastMode
		core.Go(h.env, func() (json.RawMessage, error) {
			return h.env.Host.Call("chat.ghosts.models", map[string]string{})
		}, func(rows json.RawMessage, err error) {
			if err != nil {
				p.loadErr = err.Error()
			} else {
				p.value = string(rows)
			}
		})
	}
	var rows []row
	_ = json.Unmarshal([]byte(p.value), &rows)
	open := true
	if kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: first(p.parsed.GhostName, text("title")), Width: 460, Cancel: text("cancel"), Confirm: text("allow"), ConfirmDisabled: p.model.Model == "" || p.loadErr != "",
		Body: func() {
			kit.InlineError(c, p.loadErr)
			groups := []widgets.ModelGroup{}
			var efforts []string
			for _, model := range rows {
				index := -1
				for i := range groups {
					if groups[i].ID == model.ProviderID {
						index = i
						break
					}
				}
				if index < 0 {
					groups = append(groups, widgets.ModelGroup{ID: model.ProviderID, Name: first(model.ProviderName, model.ProviderID)})
					index = len(groups) - 1
				}
				groups[index].Models = append(groups[index].Models, widgets.ModelEntry{Value: widgets.SelectionValue(widgets.AISelection{Provider: model.ProviderID, Model: model.ID}), ID: model.ID, Name: model.Name, Label: first(model.Name, model.ID)})
				if p.model.Provider == model.ProviderID && p.model.Model == model.ID {
					efforts = model.Efforts
				}
			}
			chosen, selected := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: h.env, Groups: groups, Selected: widgets.SelectionValue(p.model), Label: text("pickModel")}, Placeholder: text("pickModel")})
			if selected {
				if model, ok := widgets.ParseSelectionValue(chosen); ok {
					p.model = model
					p.effort = ""
				}
			}
			options := []kit.SelectOption{{Value: "", Label: i18n.T("nativeOneBot.defaultEffort")}}
			seen := map[string]bool{"": true}
			for _, effort := range append(efforts, p.effort) {
				if !seen[effort] {
					options = append(options, kit.SelectOption{Value: effort, Label: effort})
					seen[effort] = true
				}
			}
			ui.Field(c, i18n.T("nativeOneBot.effort"), func() { kit.Select(c, &p.effort, options) })
			kit.Toggle(c, &p.fast, i18n.T("nativeOneBot.fastMode"), "")
		}}) {
		h.answer(p, map[string]any{"requestId": picker.RequestID, "route": map[string]any{"providerId": p.model.Provider, "agentKind": "pi", "model": p.model.Model, "effort": p.effort, "fastMode": p.fast}})
	} else if !open {
		h.answer(p, map[string]any{"requestId": picker.RequestID, "cancelled": true})
	}
}

// installDialog inspects a package the Agent packed and lets the user decide with
// its real permissions, as when they choose a package themselves.
func (h *Host) installDialog(c *ui.Context, p *prompt) {
	type inspected struct {
		Token, Name, Version string
		Update               bool
		Permissions          []pluginchecks.Permission `json:"permissions"`
	}
	if !p.started {
		p.started = true
		core.Go(h.env, func() (json.RawMessage, error) {
			var value json.RawMessage
			err := h.env.Host.CallInto("chat.ghosts.inspect", map[string]string{"path": p.parsed.Path, "language": i18n.Locale()}, &value)
			return value, err
		}, func(value json.RawMessage, err error) {
			if err != nil {
				if h.env.Toast != nil {
					h.env.Toast(err.Error())
				}
				h.answer(p, map[string]any{"ok": false})
				return
			}
			p.value = string(value)
		})
	}
	var info inspected
	if p.value == "" || json.Unmarshal([]byte(p.value), &info) != nil {
		return
	}
	open := true
	if kit.Dialog(c.Key(p.ID), &open, kit.DialogOptions{Title: text("installTitle", "name", info.Name, "version", info.Version), Width: 520, Confirm: text("install"), Cancel: text("cancel"), Busy: p.installing, ConfirmDisabled: !pluginchecks.PermissionsKnown(info.Permissions),
		Body: func() {
			ui.Text(c, text("installBody"))
			pluginchecks.PermissionDetails(c, info.Permissions)
		}}) {
		p.installing = true
		token := info.Token
		core.Go(h.env, func() (json.RawMessage, error) {
			var value json.RawMessage
			err := h.env.Host.CallInto("chat.ghosts.install", map[string]any{"token": token, "confirmed": true}, &value)
			return value, err
		}, func(value json.RawMessage, err error) {
			if err != nil {
				if h.env.Toast != nil {
					h.env.Toast(err.Error())
				}
				h.answer(p, map[string]any{"ok": false})
				return
			}
			action := "installed"
			if info.Update {
				action = "updated"
			}
			h.answer(p, map[string]any{"ok": true, "action": action, "ghost": value})
		})
	} else if !open {
		h.answer(p, map[string]any{"ok": false})
	}
}

// Layout and Navigation are narrow shell seams. Pages are actual native
// containers/routes; confirmations and standard media lightboxes remain in View.
func (h *Host) Layout(c *ui.Context, onChatRoute bool, content func()) {
	if h.pages.HasMainPage() {
		h.FocusSession("")
	}
	h.pages.Layout(c, onChatRoute, content)
}
func (h *Host) Navigation(c *ui.Context, collapsed bool) { h.pages.Navigation(c, collapsed) }
func (h *Host) LeaveMain()                               { h.pages.LeaveMain() }
func (h *Host) OpenPage(plugin, page string)             { h.pages.Open(plugin, page) }
func (h *Host) OpenTaskPage(plugin, session string)      { h.pages.OpenTask(plugin, session) }
func (h *Host) TakePreviews(session string) []json.RawMessage {
	rows := h.previewRequests[session]
	delete(h.previewRequests, session)
	return rows
}

// Recommendation is a task an enabled plugin suggests for a new conversation.
type Recommendation struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Prompt   string `json:"prompt"`
}

// Recommendations reads the list off the UI thread, once per caller's refresh.
func (h *Host) Recommendations(done func([]Recommendation)) {
	h.call("recommendations", map[string]string{"language": i18n.Locale()}, func(value json.RawMessage, err error) {
		var rows []Recommendation
		if err == nil {
			_ = json.Unmarshal(value, &rows)
		}
		done(rows)
	})
}

// Call asks the plugin host one question off the UI thread.
func (h *Host) call(method string, params any, done func(json.RawMessage, error)) {
	core.Go(h.env, func() (json.RawMessage, error) {
		var value json.RawMessage
		err := h.env.Host.CallInto("chat.ghosts."+method, params, &value)
		return value, err
	}, done)
}

package chat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Native port of the cards a teammate's messages carry: BotCollaborationCard
// (background tasks and their receipts), BotDirectMessageCard and
// VoiceCallTranscript, with the data of onebotTaskView and onebotTaskPrs.
// Tracked as chat-bot-cards in third_party/cindy/native-ports.json.

// botHost is a OneBot Host method; plainChatMethod an ordinary Chat one, whatever
// scope this page's own conversation uses.
func botHost(name string) string         { return "chat." + "onebot." + name }
func plainChatMethod(name string) string { return "chat." + name }

// botCollaboration is Cindy's BotCollaborationMeta and the card's text.
type botCollaboration struct {
	V                   int    `json:"v"`
	Role                string `json:"role"`
	DelegationID        string `json:"delegationId"`
	FromBotID           string `json:"fromBotId"`
	ParentSessionID     string `json:"parentSessionId"`
	ChildSessionID      string `json:"childSessionId"`
	Objective           string `json:"objective"`
	InterjectionPreview string `json:"interjectionPreview"`
	Result              *struct {
		Status     string `json:"status"`
		Title      string `json:"title"`
		Text       string `json:"text"`
		Error      string `json:"error"`
		WorkingDir string `json:"workingDir"`
		Artifacts  []struct {
			AbsolutePath string `json:"absolutePath"`
		} `json:"artifacts"`
	} `json:"result"`
}

// botDelegation is the part of Cindy's BotDelegationView the card reads.
type botDelegation struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	ChildSessionID string `json:"childSessionId"`
	CreatedAt      int64  `json:"createdAt"`
	AcceptedAt     *int64 `json:"acceptedAt"`
	CompletedAt    *int64 `json:"completedAt"`
	ResultSummary  string `json:"resultSummary"`
	LastError      string `json:"lastError"`
	Artifacts      []struct {
		AbsolutePath string `json:"absolutePath"`
	} `json:"artifacts"`
	PendingInteraction *struct {
		Summary string `json:"summary"`
	} `json:"pendingInteraction"`
}

type botPRRef struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
}

type botPRStatus struct {
	OK       bool   `json:"ok"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
	Status   string `json:"status"`
}

func (r botPRRef) key() string {
	return strings.ToLower(r.Owner) + "/" + strings.ToLower(r.Repo) + "#" + fmt.Sprint(r.PRNumber)
}

func (r botPRRef) url() string {
	return "https://github.com/" + url.PathEscape(r.Owner) + "/" + url.PathEscape(r.Repo) + "/pull/" + fmt.Sprint(r.PRNumber)
}

const (
	botTaskPollInterval = 1500 * time.Millisecond
	// botPRRefresh stays above the host's 60-second cache, as Cindy's does, so
	// each tick really reaches GitHub.
	botPRRefresh     = 90 * time.Second
	botPRMaxQueries  = 3
	botNoteMaxLength = 4000
)

// botPRState is the pull requests of one background task.
type botPRState struct {
	refs     []botPRRef
	statuses map[string]botPRStatus
	lastGood map[string]string
	failed   bool
	loading  bool
	cancel   func()
}

// botCardState is what a card keeps across frames.
type botCardState struct {
	opened, resultOpen, noteOpen, pending bool
	note, err                             string
}

// botTaskStore is every delegation of the conversation the cards show. One read
// serves all cards, and it stops polling as soon as no task is active.
type botTaskStore struct {
	parent   string
	rows     map[string]botDelegation
	catalog  Catalog
	activity map[string]Activity
	used     bool
	resolved bool
	stale    bool
	loading  bool
	again    bool
	cancel   func()
	cards    map[string]*botCardState
	prs      map[string]*botPRState
}

func (s *botTaskStore) refresh(p *Chat) {
	if !s.used || s.parent == "" || p.ui.closed {
		return
	}
	if s.loading {
		s.again = true
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.loading = true
	parent := s.parent
	type read struct {
		tasks    []botDelegation
		catalog  Catalog
		activity map[string]Activity
	}
	core.Go(p.env, func() (read, error) {
		var r read
		var tasks struct {
			Delegations []botDelegation `json:"delegations"`
		}
		if err := p.env.Host.CallInto(botHost("tasks.list"), map[string]any{"callerSessionId": parent}, &tasks); err != nil {
			return r, err
		}
		// A child task belongs to the ordinary Chat catalog, not this chat's scope.
		if err := p.env.Host.CallInto(plainChatMethod("catalog"), map[string]any{}, &r.catalog); err != nil {
			return r, err
		}
		if err := p.env.Host.CallInto(plainChatMethod("activity"), map[string]any{}, &r.activity); err != nil {
			return r, err
		}
		r.tasks = tasks.Delegations
		return r, nil
	}, func(r read, err error) {
		s.loading = false
		if p.ui.closed || s.parent != parent {
			return
		}
		s.resolved = true
		s.stale = err != nil
		active := err != nil
		if err == nil {
			s.rows = map[string]botDelegation{}
			for _, row := range r.tasks {
				s.rows[row.ID] = row
				active = active || botDelegationActive(row.Status)
			}
			s.catalog, s.activity = r.catalog, r.activity
		}
		if s.again {
			s.again = false
			s.refresh(p)
		} else if active {
			s.cancel = p.after(botTaskPollInterval, func() {
				s.cancel = nil
				s.refresh(p)
			})
		}
	})
}

func (s *botTaskStore) close() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	for _, pr := range s.prs {
		if pr.cancel != nil {
			pr.cancel()
			pr.cancel = nil
		}
	}
}

// use starts reads the first time a card of this conversation is drawn.
func (s *botTaskStore) use(p *Chat, parent string) {
	if s.parent != parent {
		s.close()
		*s = botTaskStore{parent: parent}
	}
	if !s.used {
		s.used = true
		s.refresh(p)
	}
}

func (s *botTaskStore) card(id string) *botCardState {
	if s.cards == nil {
		s.cards = map[string]*botCardState{}
	}
	card := s.cards[id]
	if card == nil {
		card = &botCardState{}
		s.cards[id] = card
	}
	return card
}

// pullRequests reads a shown task's PRs and keeps them current while it is shown.
// No hidden polling: the timer is only armed from a frame that drew the card.
func (s *botTaskStore) pullRequests(p *Chat, child string) *botPRState {
	if s.prs == nil {
		s.prs = map[string]*botPRState{}
	}
	state := s.prs[child]
	if state == nil {
		state = &botPRState{statuses: map[string]botPRStatus{}, lastGood: map[string]string{}}
		s.prs[child] = state
	}
	if state.loading || state.cancel != nil {
		return state
	}
	state.loading = true
	type read struct {
		refs     []botPRRef
		statuses []botPRStatus
	}
	core.Go(p.env, func() (read, error) {
		var r read
		if err := p.env.Host.CallInto(botHost("tasks.pr.refs"), map[string]string{"childSessionId": child}, &r.refs); err != nil {
			return r, err
		}
		if len(r.refs) > 0 {
			if err := p.env.Host.CallInto(botHost("tasks.pr.statuses"), map[string]string{"childSessionId": child}, &r.statuses); err != nil {
				return r, err
			}
		}
		return r, nil
	}, func(r read, err error) {
		state.loading = false
		if p.ui.closed {
			return
		}
		state.failed = err != nil
		if err == nil {
			state.refs = r.refs[:min(len(r.refs), botPRMaxQueries)]
			state.statuses = map[string]botPRStatus{}
			for _, status := range r.statuses {
				key := botPRRef{status.Owner, status.Repo, status.PRNumber}.key()
				state.statuses[key] = status
				if status.OK {
					state.lastGood[key] = status.Status
				}
			}
		}
		state.cancel = p.after(botPRRefresh, func() { state.cancel = nil })
	})
	return state
}

func botDelegationActive(status string) bool {
	return status == "queued" || status == "running" || status == "waiting"
}

// botDuration is formatBotCollaborationDuration: seconds, minutes, hours and
// minutes, then days, with the units from the catalog.
func botDuration(from, to int64) string {
	seconds := max(int64(0), (to-from)/1000)
	minutes := seconds / 60
	switch {
	case seconds < 60:
		return botT("bots.collab.duration.seconds", "n", seconds)
	case minutes < 60:
		return botT("bots.collab.duration.minutes", "n", minutes)
	case seconds >= 86400:
		return botT("bots.collab.duration.daysHoursMinutes", "d", seconds/86400, "h", seconds%86400/3600, "m", minutes%60)
	}
	return botT("bots.collab.duration.hoursMinutes", "h", minutes/60, "m", minutes%60)
}

// botCardFor is the card a message carries, or false for an ordinary message.
func (p *Chat) botCardFor(c *ui.Context, m model.Message) bool {
	meta := readBotMeta(m)
	switch {
	case len(meta.VoiceCall) > 0:
		return p.voiceCallCard(c, meta.VoiceCall)
	case len(meta.BotCollaboration) > 0:
		var card botCollaboration
		if json.Unmarshal(meta.BotCollaboration, &card) != nil || card.V != 1 || card.DelegationID == "" {
			return false
		}
		switch card.Role {
		case "delegation-request":
			p.botCardColumn(c, func() { p.botTaskCard(c, card) })
		case "delegation-result":
			if card.Result == nil {
				return false
			}
			p.botCardColumn(c, func() { p.botTaskCard(c, card) })
		case "interjection":
			p.botCardColumn(c, func() { botInterjectionTrace(c, card) })
		default:
			return false
		}
		return true
	case len(meta.BotDirectMessage) > 0:
		var direct struct {
			Direction, PeerBotID, PeerBotName, ViewerBotID, ThreadID string
		}
		if json.Unmarshal(meta.BotDirectMessage, &direct) != nil || direct.ThreadID == "" {
			return false
		}
		p.botCardColumn(c, func() {
			name := first(direct.PeerBotName, direct.PeerBotID)
			key := "bots.directMessage.receivedFrom"
			if direct.Direction == "sent" {
				key = "bots.directMessage.sentTo"
			}
			if kit.GhostButton(c.Key("direct:"+direct.ThreadID), botT(key, "name", name), icons.ArrowLeftRight, false).Tooltip(botT("bots.directMessage.open")).Clicked() {
				target, _ := json.Marshal(map[string]string{"botId": direct.ViewerBotID, "directThread": direct.ThreadID})
				p.env.Open("onebot", core.Intent{Requested: true, Options: target})
			}
		})
		return true
	}
	return false
}

// botCardColumn keeps a card in the teammate's reply column.
func (p *Chat) botCardColumn(c *ui.Context, body func()) {
	if p.bot == nil {
		ui.Column(c).FillWidth().Children(body)
		return
	}
	p.botReply(c, false, body)
}

func botInterjectionTrace(c *ui.Context, card botCollaboration) {
	t := c.Theme()
	text := botT("bots.collab.messageSent")
	if card.InterjectionPreview != "" {
		text = botT("bots.collab.messageSentWithText", "text", card.InterjectionPreview)
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		ui.Icon(c, icons.Megaphone).FontSize(13).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, text).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).Grow(1).MinWidth(0)
	})
}

// botCardFacts are what the card's states derive from its row and receipt.
type botCardFacts struct {
	state, statusLabel        string
	live, active, hasResult   bool
	title, resultText, errMsg string
	files                     []string
	child, workingDir         string
	elapsed, took             string
}

func (s *botTaskStore) facts(card botCollaboration, now int64) botCardFacts {
	row, hasRow := s.rows[card.DelegationID]
	frozen := card.Result
	child := card.ChildSessionID
	if hasRow && row.ChildSessionID != "" {
		child = row.ChildSessionID
	}
	activity, hasActivity := s.activity[child]
	running := hasActivity && (activity.Phase == "running" || activity.Phase == "needs-interaction")
	f := botCardFacts{child: child}
	switch {
	case frozen != nil && frozen.Status == "completed":
		f.state = "completed"
	case frozen != nil && frozen.Status == "cancelled":
		f.state = "cancelled"
	case frozen != nil:
		f.state = "failed"
	case hasRow && row.Status == "waiting":
		f.state = "waiting"
	case running || hasRow && row.Status == "running":
		f.state = "running"
	case !hasRow && s.resolved:
		f.state = "unknown"
	case !hasRow || row.Status == "queued":
		f.state = "starting"
	case row.Status == "completed":
		f.state = "completed"
	case row.Status == "failed" || row.Status == "timed-out":
		f.state = "failed"
	case row.Status == "cancelled":
		f.state = "cancelled"
	default:
		f.state = "unknown"
	}
	f.live = f.state == "starting" || f.state == "running" || f.state == "waiting"
	f.active = frozen == nil && hasRow && botDelegationActive(row.Status)
	switch {
	case frozen != nil:
		f.statusLabel = botT("bots.collab.status." + frozen.Status)
	case f.state == "unknown":
		f.statusLabel = botT("bots.collab.status.unknown")
	case f.state == "starting":
		f.statusLabel = botT("bots.collab.status.queued")
	case f.state == "failed" && hasRow:
		f.statusLabel = botT("bots.collab.status." + row.Status)
	default:
		f.statusLabel = botT("bots.collab.status." + f.state)
	}
	sessionTitle, sessionDir := "", ""
	if conversation := s.catalog.conversation(child); conversation != nil {
		sessionTitle, sessionDir = conversation.Title, conversation.Directory
	}
	firstLine, _, _ := strings.Cut(strings.TrimSpace(card.Objective), "\n")
	if frozen != nil {
		f.title = first(strings.TrimSpace(frozen.Title), strings.TrimSpace(row.Title), sessionTitle, firstLine, botT("bots.collab.backgroundTask"))
		f.resultText, f.errMsg, f.workingDir = frozen.Text, frozen.Error, first(frozen.WorkingDir, sessionDir)
		for _, file := range frozen.Artifacts {
			f.files = append(f.files, file.AbsolutePath)
		}
	} else {
		f.title = first(sessionTitle, row.Title, firstLine, botT("bots.collab.backgroundTask"))
		f.resultText, f.workingDir = row.ResultSummary, sessionDir
		if _, rest, ok := strings.Cut(row.LastError, ": "); ok && strings.ToUpper(row.LastError[:strings.Index(row.LastError, ": ")]) == row.LastError[:strings.Index(row.LastError, ": ")] {
			f.errMsg = rest
		} else {
			f.errMsg = row.LastError
		}
		for _, file := range row.Artifacts {
			if file.AbsolutePath != "" {
				f.files = append(f.files, file.AbsolutePath)
			}
		}
	}
	f.hasResult = frozen != nil || f.state == "completed" || f.state == "failed" || f.state == "cancelled"
	if frozen == nil && hasRow {
		started := row.CreatedAt
		if row.AcceptedAt != nil {
			started = *row.AcceptedAt
		}
		if f.live {
			f.elapsed = botDuration(started, now)
		} else if row.CompletedAt != nil {
			f.took = botDuration(row.CreatedAt, *row.CompletedAt)
		}
	}
	return f
}

// botTaskCard is the one card for a background task: state changes in place, a
// receipt keeps its frozen status, text, files and error.
func (p *Chat) botTaskCard(c *ui.Context, data botCollaboration) {
	t, pal := c.Theme(), kit.Colors(c)
	parent := first(p.activeID, data.ParentSessionID)
	store := &p.botTasks
	store.use(p, parent)
	state := store.card(data.Role + ":" + data.DelegationID)
	f := store.facts(data, time.Now().UnixMilli())
	if f.live && data.Result == nil {
		// The elapsed time ticks only while the task works.
		c.After(time.Second)
	}
	canOpen := f.child != ""
	var prs *botPRState
	if f.child != "" {
		prs = store.pullRequests(p, f.child)
	}
	row := store.rows[data.DelegationID]
	ui.Column(c.Key("task:"+data.Role+":"+data.DelegationID)).FillWidth().MaxWidth(560).Gap(6).Padding(10, 12).Radius(12).Border(1, pal.Hairline).Background(pal.Panel).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			botStatusDot(c, f.state)
			title := ui.ButtonBase(c.Key("title")).Grow(1).Basis(0).MinWidth(0).Justify(ui.Start).Tooltip(f.statusLabel).Label(f.statusLabel + " " + f.title).Disabled(!canOpen)
			title.Children(func() { ui.Text(c, f.title).FontWeight(500).SingleLine().Ellipsis("…").MinWidth(0) })
			if title.Clicked() {
				p.botOpenTask(f.child)
			}
			parts := []string{botT("bots.collab.backgroundTask")}
			if f.took != "" {
				parts = append(parts, botT("bots.collab.tookTime", "duration", f.took))
			}
			ui.Text(c, strings.Join(parts, " · ")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).MaxWidthPercent(55)
		})
		botCardLine(c, f, row, state.resultOpen)
		if (store.stale || prs != nil && prs.failed) && len(store.rows) > 0 {
			kit.Notice(c, kit.Caution, "", botT("bots.collab.stale"), nil)
		}
		if state.noteOpen {
			note := ui.TextInput(c.Key("note"), &state.note).FillWidth().Placeholder(botT("bots.collab.addNotePlaceholder")).Label(botT("bots.collab.addNote")).Disabled(state.pending)
			note.OnTextKey(func(k ui.TextKey) bool {
				switch k.Key {
				case ui.KeyEscape:
					state.noteOpen = false
					return true
				case ui.KeyEnter:
					state.note = k.Text()
					p.submitBotNote(data, f.child, state)
					return true
				}
				return false
			})
		}
		if state.resultOpen && f.hasResult {
			p.botCardResult(c, data, f)
		}
		ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			p.botCardChips(c, data, f, state, prs, canOpen)
		})
		kit.InlineError(c, state.err)
	})
}

func botStatusDot(c *ui.Context, state string) {
	t := c.Theme()
	color := t.TextMuted
	switch state {
	case "starting", "running":
		color = statusAwaiting(t)
	case "completed":
		color = t.TextMuted
	case "failed":
		color = statusError(t)
	case "waiting":
		color = statusAwaiting(t)
	}
	ui.Column(c).Size(8, 8).Radius(4).Background(color).Shrink(0)
}

func botCardLine(c *ui.Context, f botCardFacts, row botDelegation, open bool) {
	t := c.Theme()
	small := func(text string, lines int, muted bool) {
		line := ui.Text(c, text).FontSize(kit.HelpSize(c)).MinWidth(0).MaxLines(lines).Ellipsis("…")
		if muted {
			line.TextColor(t.TextMuted)
		}
	}
	switch {
	case f.state == "starting":
		small(botT("bots.collab.status.queued"), 1, true)
	case f.state == "running":
		text := botT("ccAgent.agentStatus.processing")
		if f.elapsed != "" {
			text += " · " + botT("bots.collab.elapsed", "duration", f.elapsed)
		}
		small(text, 1, true)
	case f.state == "waiting":
		if row.PendingInteraction != nil && strings.TrimSpace(row.PendingInteraction.Summary) != "" {
			small(botT("bots.collab.awaitingConfirm", "summary", strings.TrimSpace(row.PendingInteraction.Summary)), 2, true)
		} else {
			small(botT("bots.collab.awaitingConfirmGeneric"), 2, true)
		}
	case f.state == "completed" && strings.TrimSpace(f.resultText) != "" && !open:
		small(strings.TrimSpace(f.resultText), 2, false)
	case f.state == "failed" && f.errMsg != "" && !open:
		kit.InlineError(c, f.errMsg)
	}
}

// botCardResult shows the task's own result: links in it belong to the task's
// directory, which may differ from this chat's.
func (p *Chat) botCardResult(c *ui.Context, data botCollaboration, f botCardFacts) {
	t := c.Theme()
	ui.Column(c).FillWidth().Height(1).Background(t.Border)
	key := "bot-result:" + data.DelegationID
	if strings.TrimSpace(f.resultText) != "" {
		o := &markdown.Options{WrapCode: true, OnLink: func(destination string) { p.openBotResultLink(destination, f.workingDir, key) }}
		markdown.View(c, f.resultText, mathview.For(p.env).Apply(o)).FillWidth()
	} else {
		ui.Text(c, botT("bots.collab.noWrittenResult")).TextColor(t.TextMuted)
	}
	for _, path := range f.files {
		path := path
		if kit.GhostButton(c.Key(path), filepath.Base(path), icons.FileText, false).Tooltip(path).Clicked() {
			p.openBotResultLink(path, f.workingDir, key)
		}
	}
	if f.errMsg != "" {
		p.disclosure(c, key+":details", icons.TriangleAlert, botT("appError.details"), "", func() {
			kit.InlineError(c, f.errMsg)
		})
	}
}

// openBotResultLink resolves a result's link against the task's directory.
func (p *Chat) openBotResultLink(destination, directory, surface string) {
	target := model.TargetOf(destination, first(directory, p.directory()), p.attachments())
	if !target.Local() {
		if safeLink(destination) {
			p.ui.services.OpenURL(destination)
		}
		return
	}
	p.openLink(target.Path, surface)
}

func (p *Chat) botOpenTask(sessionID string) {
	p.env.Open("chat", core.Intent{Requested: true, Conversation: sessionID})
}

func botChip(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	return kit.GhostButton(c.Key(key), label, icon, false)
}

func (p *Chat) botCardChips(c *ui.Context, data botCollaboration, f botCardFacts, state *botCardState, prs *botPRState, canOpen bool) {
	if f.state == "waiting" && canOpen && botChip(c, "look", botT("bots.collab.lookAt"), icons.Eye).Clicked() {
		p.botOpenTask(f.child)
	}
	if (f.state == "running" || f.state == "starting") && canOpen && botChip(c, "watch", botT("bots.collab.watchWork"), icons.SquareArrowOutUpRight).Clicked() {
		p.botOpenTask(f.child)
	}
	if f.state == "running" && canOpen && data.Result == nil && botChip(c, "note", botT("bots.collab.addNote"), icons.Megaphone).Disabled(state.pending).Clicked() {
		state.noteOpen = !state.noteOpen
	}
	if !f.live && len(f.files) > 0 && botChip(c, "files", botT("bots.collab.artifactCount", "count", len(f.files)), icons.FileText).Clicked() {
		state.resultOpen = true
	}
	if prs != nil {
		for _, ref := range prs.refs {
			ref := ref
			icon := icons.GitPullRequest
			status := prs.lastGood[ref.key()]
			switch status {
			case "merged":
				icon = icons.GitMerge
			case "closed":
				icon = icons.GitPullRequestClosed
			case "draft":
				icon = icons.GitPullRequestDraft
			}
			if botChip(c, "pr:"+ref.key(), fmt.Sprintf("PR #%d", ref.PRNumber), icon).Tooltip(fmt.Sprintf("%s/%s #%d", ref.Owner, ref.Repo, ref.PRNumber)).Clicked() {
				p.ui.services.OpenURL(ref.url())
			}
		}
	}
	ui.Spacer(c)
	if !f.live && f.hasResult {
		label, icon := botT("bots.collab.expandDetails"), icons.ChevronDown
		if f.state == "completed" {
			label = botT("bots.collab.expandResult")
		}
		if state.resultOpen {
			label, icon = botT("bots.collab.collapseDetails"), icons.ChevronUp
			if f.state == "completed" {
				label = botT("bots.collab.collapseResult")
			}
		}
		if botChip(c, "result", label, icon).Clicked() {
			state.resultOpen = !state.resultOpen
		}
	}
	if !f.live && canOpen && botChip(c, "watch-done", botT("bots.collab.watchWork"), icons.SquareArrowOutUpRight).Clicked() {
		p.botOpenTask(f.child)
	}
	if f.active && botChip(c, "stop", botT("bots.collab.stop"), icons.Square).Disabled(state.pending || p.botTasks.parent == "").Clicked() {
		p.stopBotTask(data, state)
	}
}

// stopBotTask cancels the task through the caller's own delegation.
func (p *Chat) stopBotTask(data botCollaboration, state *botCardState) {
	state.pending, state.err = true, ""
	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	parent := p.botTasks.parent
	core.Do(p.env, func() error {
		return p.env.Host.CallInto(botHost("tasks.stop"), map[string]any{"callerSessionId": parent, "delegationId": data.DelegationID, "mode": "cancel"}, &result)
	}, func(err error) {
		state.pending = false
		switch {
		case err != nil:
			state.err = botT("bots.collab.actionFailed")
		case !result.OK:
			state.err = first(result.Message, botT("bots.collab.actionFailed"))
		}
		p.botTasks.refresh(p)
	})
}

// submitBotNote adds a line to the original task as if typing in it. The line is
// delivered before the quiet trace, so a failed trace never turns a sent note
// into an error.
func (p *Chat) submitBotNote(data botCollaboration, child string, state *botCardState) {
	text := strings.TrimSpace(state.note)
	if text == "" || child == "" || state.pending || utf16Len(text) > botNoteMaxLength {
		return
	}
	state.pending, state.err = true, ""
	parent := p.botTasks.parent
	var receipt struct {
		Accepted bool `json:"accepted"`
	}
	core.Do(p.env, func() error {
		err := p.env.Host.CallInto(plainChatMethod("prompt"), map[string]any{"id": child, "text": text, "requestId": newID(), "preserveDraft": true}, &receipt)
		if err == nil && receipt.Accepted && parent != "" {
			_ = p.env.Host.CallInto(botHost("tasks.trace"), map[string]any{"callerSessionId": parent, "delegationId": data.DelegationID, "text": text}, nil)
		}
		return err
	}, func(err error) {
		state.pending = false
		if err != nil || !receipt.Accepted {
			state.err = botT("bots.collab.actionFailed")
			return
		}
		state.note, state.noteOpen = "", false
		p.notice = botT("bots.collab.addNoteSent")
	})
}

// voiceCallCard is a saved call: its turns and what each handed to the task. The
// open call owns its live record, so that record is not repeated here.
func (p *Chat) voiceCallCard(c *ui.Context, raw json.RawMessage) bool {
	var record struct {
		CallID string `json:"callId"`
		Turns  []struct {
			ID, Role, Text string
			Failed         bool
		} `json:"turns"`
		Handoffs []struct{ ID, InputID, State string } `json:"handoffs"`
	}
	if json.Unmarshal(raw, &record) != nil {
		return false
	}
	content := len(record.Handoffs) > 0
	for _, turn := range record.Turns {
		content = content || strings.TrimSpace(turn.Text) != ""
	}
	if !content || p.bot != nil && p.bot.ActiveCall != nil && p.bot.ActiveCall() == record.CallID {
		return true
	}
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c.Key("voice-call:"+record.CallID)).FillWidth().Gap(8).Padding(10, 12).Radius(12).Border(1, pal.Hairline).Background(pal.Panel).Label(botT("bots.voiceMode.callRecord")).Children(func() {
		for _, turn := range record.Turns {
			ui.Column(c.Key(turn.ID)).FillWidth().Gap(4).Children(func() {
				ui.Text(c, botT(map[bool]string{true: "bots.voiceMode.you", false: "bots.voiceMode.companion"}[turn.Role == "user"])).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
				ui.Text(c, turn.Text).LineHeight(1.5).Selectable().MinWidth(0)
				var handoff *struct{ ID, InputID, State string }
				for i := range record.Handoffs {
					if record.Handoffs[i].InputID == turn.ID {
						handoff = &record.Handoffs[i]
					}
				}
				switch {
				case handoff == nil && turn.Failed:
					kit.InlineError(c, botT("bots.voiceMode.callSendFailed"))
				case handoff != nil && (handoff.State == "accepted" || handoff.State == "completed"):
					id := handoff.ID
					if kit.GhostButton(c.Key("task"), botT("bots.voiceMode.viewTaskProgress")+" · "+botT("bots.voiceMode.handoff."+handoff.State), icons.ListTodo, false).Clicked() {
						p.locateVoiceTask(id)
					}
				case handoff != nil:
					kit.Status(c, botT("bots.voiceMode.handoff."+handoff.State))
				}
			})
		}
	})
	return true
}

// locateVoiceTask goes to the reply of the turn a call handed over.
func (p *Chat) locateVoiceTask(clientID string) {
	for _, m := range p.view.Messages {
		if m.Role == "assistant" && m.TurnID == clientID && m.ID != "" {
			p.JumpToMessage(m.ID, m.ID)
			return
		}
	}
	p.JumpToMessage(clientID, clientID)
}

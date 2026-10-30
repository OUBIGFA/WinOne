//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type ServiceOptions struct {
	Bun, Worker string
	// Configure supplies an ephemeral model endpoint and its release function.
	// Credentials never enter navigation metadata or desktop responses.
	Configure func(Conversation) (map[string]any, func(), error)
	// ResourceConfig preserves owner-specific resource choices when the model
	// cannot be configured. It supplies metadata inputs without starting tools.
	ResourceConfig func(context.Context, Conversation) (map[string]any, error)
	// OnTurnSettled is a nonblocking host invalidation, after the Pi receipt.
	// OneBot uses it to count/learn completed replies outside the agent loop.
	OnTurnSettled func(string)
	// OnActivity reports that a conversation's activity snapshot changed
	// (phase, pending interactions, turn or tool in flight). It runs on its own
	// goroutine; OneBot's teammate follow-up reads ActivitySnapshots.
	OnActivity func(string)
}
type Event struct {
	Sequence uint64          `json:"sequence"`
	Data     json.RawMessage `json:"data"`
}
type EventPage struct {
	Events    []Event           `json:"events"`
	Cursor    uint64            `json:"cursor"`
	Reset     bool              `json:"reset"`
	More      bool              `json:"more"`
	Running   bool              `json:"running"`
	Status    string            `json:"status"`
	Error     string            `json:"error,omitempty"`
	Approvals []json.RawMessage `json:"approvals"`
}
type ConversationActivity struct {
	Phase     string `json:"phase"`
	Attention bool   `json:"attention"`
	Paused    bool   `json:"paused"`
}

type liveSession struct {
	gate              chan struct{}
	mu                sync.Mutex
	runner            *Runner
	release           func()
	owners            map[uint32]bool
	executionOwners   int // Owner zero is a reference-counted scheduler execution lease.
	events            []Event
	bytes             int
	sequence          uint64
	running, closed   bool
	backgroundRunning bool // Live durable children, even after the parent reply settles.
	sessionPaused     bool // Durable scheduling choice, independent of application shutdown.
	status, failure   string
	initialized       bool
	modelError        string
	approvals         map[string]json.RawMessage
	changed           chan struct{}
	activeRequest     string
	// Activity projection for teammate follow-up (see activity_windows.go).
	activity   activityState
	onActivity func()
}

var ErrBusy = errors.New("请等待当前任务结束或先停止")

func (p *liveSession) notifyLocked() {
	if p.changed != nil {
		close(p.changed)
	}
	p.changed = make(chan struct{})
	// Every state change passes here: the activity projection observes them all.
	if p.observeActivityLocked() && p.onActivity != nil {
		go p.onActivity()
	}
}

type sessionOutcome struct {
	status, failure string
	paused          bool
	activity        activityState
}

type Service struct {
	mu            sync.Mutex
	ctx           context.Context
	store         *Store
	options       ServiceOptions
	sessions      map[string]*liveSession
	outcomes      map[string]sessionOutcome // Terminal metadata survives retirement, not the worker itself.
	paused        bool
	exports       chan struct{}
	archiveRunner *Runner
}

func NewService(ctx context.Context, store *Store, options ServiceOptions) *Service {
	return &Service{ctx: ctx, store: store, options: options, sessions: map[string]*liveSession{}, outcomes: map[string]sessionOutcome{}, exports: make(chan struct{}, 1)}
}

// Activity is a cheap control-plane read. It never acquires a session gate,
// contacts a worker, loads history or starts unopened conversations.
func (s *Service) Activity() map[string]ConversationActivity {
	s.mu.Lock()
	sessions := make(map[string]*liveSession, len(s.sessions))
	for id, p := range s.sessions {
		sessions[id] = p
	}
	result := make(map[string]ConversationActivity, len(s.outcomes)+len(sessions))
	for id, outcome := range s.outcomes {
		result[id] = (&liveSession{status: outcome.status, failure: outcome.failure, sessionPaused: outcome.paused}).activityLocked()
	}
	s.mu.Unlock()
	for id, p := range sessions {
		p.mu.Lock()
		result[id] = p.activityLocked()
		p.mu.Unlock()
	}
	// Prune deleted conversations using in-memory navigation only: no history
	// reads, worker startup or disk I/O is added to the sidebar poll.
	if s.store != nil {
		known := make(map[string]bool)
		for _, conversation := range s.store.Snapshot().Conversations {
			known[conversation.ID] = true
		}
		s.mu.Lock()
		for id := range s.outcomes {
			if !known[id] {
				delete(s.outcomes, id)
				delete(result, id)
			}
		}
		s.mu.Unlock()
	}
	return result
}

// Caller owns p.mu. A stopped run is not a failed request even if Pi includes
// an AbortError diagnostic; pause remains a separate scheduling state.
func (p *liveSession) activityLocked() ConversationActivity {
	value := ConversationActivity{Phase: "idle", Paused: p.sessionPaused}
	switch {
	case p.status == "stopped":
		value.Phase = "interrupted"
	case p.failure != "" || p.status == "failed":
		value.Phase, value.Attention = "error", true
	case p.sessionPaused:
	case len(p.approvals) > 0:
		value.Phase = "needs-interaction"
	case p.running || p.backgroundRunning:
		value.Phase = "running"
	case p.status == "completed":
		value.Phase, value.Attention = "completed", true
	}
	return value
}

func (s *Service) newSessionLocked(id string) *liveSession {
	outcome := s.outcomes[id]
	p := &liveSession{gate: make(chan struct{}, 1), owners: map[uint32]bool{}, approvals: map[string]json.RawMessage{}, status: outcome.status, failure: outcome.failure, sessionPaused: outcome.paused, activity: outcome.activity}
	if s.options.OnActivity != nil {
		p.onActivity = func() { s.options.OnActivity(id) }
	}
	if p.status == "" {
		p.status = "idle"
	}
	s.sessions[id] = p
	return p
}

func (s *Service) session(id string) (*liveSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused {
		return nil, errors.New("Chat 正在退出，请稍后重试")
	}
	p := s.sessions[id]
	if p == nil {
		return nil, errors.New("请先打开对话")
	}
	return p, nil
}
func (p *liveSession) lock(ctx context.Context) error {
	select {
	case p.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *liveSession) unlock() { <-p.gate }

// A per-conversation gate serializes mutations. Event delivery never acquires
// that gate and never calls the runner, so a permission wait cannot deadlock it.
func (s *Service) Open(ctx context.Context, id string, owner uint32) (json.RawMessage, error) {
	c, err := s.store.Conversation(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		return nil, errors.New("Chat 正在退出")
	}
	p := s.sessions[id]
	if p == nil {
		if len(s.sessions) >= 4 {
			s.mu.Unlock()
			return nil, errors.New("请关闭一个空闲对话，或等待当前任务结束")
		}
		p = s.newSessionLocked(id)
	}
	s.mu.Unlock()
	if err = p.lock(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	closed := p.closed
	if !closed {
		if owner == 0 {
			p.executionOwners++
		} else {
			p.owners[owner] = true
		}
	}
	p.mu.Unlock()
	if closed {
		// An idle-release may retire this entry while Open waits for its gate.
		// Re-resolve the catalog/session instead of exposing a transient failure.
		s.mu.Lock()
		current := s.sessions[id]
		s.mu.Unlock()
		if current != p {
			p.unlock()
			return s.Open(ctx, id, owner)
		}
		if p.runner != nil {
			select {
			case <-p.runner.Done():
				s.retire(id, p)
				p.unlock()
				return s.Open(ctx, id, owner)
			default:
			}
		}
		p.unlock()
		return nil, errors.New("对话正在释放，请重新打开")
	}
	defer p.unlock()
	opened := false
	if owner == 0 {
		defer func() {
			if !opened {
				s.Release(id, 0)
			}
		}()
	}
	c, err = s.store.Conversation(id)
	if err != nil {
		return nil, err
	}
	if p.runner == nil {
		fail := func(err error) (json.RawMessage, error) { s.retire(id, p); return nil, err }
		if c.HasMessages {
			if _, err := os.Stat(c.SessionFile); err != nil {
				return fail(fmt.Errorf("会话文件无法读取，原记录已保留：%w", err))
			}
		}
		config, release, modelErr := s.options.Configure(c)
		p.release = release
		// An owner may reconcile its saved model/permission choice while
		// configuring the endpoint. Initialize from that committed metadata.
		c, err = s.store.Conversation(id)
		if err != nil {
			return fail(err)
		}
		runner, err := StartRunner(s.ctx, s.options.Bun, s.options.Worker, c.Directory, func(raw json.RawMessage) { s.event(id, p, raw) })
		if err != nil {
			return fail(err)
		}
		p.runner = runner
		var raw json.RawMessage
		if modelErr == nil {
			config["cwd"], config["sessionDirectory"], config["sessionFile"], config["title"], config["permission"] = c.Directory, c.SessionDirectory, c.SessionFile, c.Title, c.Permission
			raw, err = runner.Call(ctx, "init", config)
			p.initialized = err == nil
			if err != nil {
				// Resource/extension failures must not hide saved messages. Inspect
				// Pi's native history and return the failure as a visible modelError;
				// prompt stays disabled until the user repairs the configuration.
				p.modelError = err.Error()
				raw, err = s.readLocked(ctx, c, p, nil)
			}
		} else {
			p.modelError = modelErr.Error()
			raw, err = s.readLocked(ctx, c, p, nil)
		}
		if err == nil {
			err = s.remember(id, raw)
		}
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = runner.Close(closeCtx)
			cancel()
			return fail(err)
		}
		go func() {
			<-runner.Done()
			p.mu.Lock()
			unexpected := !p.closed
			p.running = false
			p.backgroundRunning = false
			if unexpected {
				p.failure = runner.failure().Error()
				p.status = "failed"
			}
			p.notifyLocked()
			p.mu.Unlock()
			if unexpected {
				go s.closeIdle(id, p)
			}
		}()
		opened = true
		return raw, nil
	}
	raw, err := s.readLocked(ctx, c, p, nil)
	if err == nil {
		err = s.remember(id, raw)
	}
	opened = err == nil
	return raw, err
}

// History stays available after a provider is removed. An uninitialized Pi
// process only reads its native session format and owns no model endpoint.
func (s *Service) readLocked(ctx context.Context, c Conversation, p *liveSession, params any) (json.RawMessage, error) {
	if p.initialized {
		return p.runner.Call(ctx, "state", params)
	}
	var raw json.RawMessage
	var err error
	if c.SessionFile == "" {
		raw = json.RawMessage(`{"messages":[],"total":0,"hasOlder":false,"savedInputs":[]}`)
	} else {
		if _, statErr := os.Stat(c.SessionFile); os.IsNotExist(statErr) && !c.HasMessages {
			raw = json.RawMessage(`{"messages":[],"total":0,"hasOlder":false,"savedInputs":[]}`)
		} else if statErr != nil {
			return nil, statErr
		} else {
			args := map[string]any{"file": c.SessionFile, "cwd": c.Directory, "directory": c.SessionDirectory}
			if supplied, ok := params.(map[string]any); ok {
				args["before"] = supplied["before"]
			}
			raw, err = p.runner.Call(ctx, "inspect", args)
			if err != nil {
				return nil, err
			}
		}
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	value["modelError"] = p.modelError
	value["permission"] = c.Permission
	return json.Marshal(value)
}
func (s *Service) remember(id string, raw json.RawMessage) error {
	var state struct {
		Resources   json.RawMessage `json:"resources"`
		File        string          `json:"file"`
		SavedInputs []string        `json:"savedInputs"`
		Total       int             `json:"total"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	// Restore exact loaded-resource grants before the renderer restores file tabs.
	// These paths come from the trusted Worker, never from a preview request.
	if state.Resources != nil {
		if err := s.store.RememberResources(id, state.Resources); err != nil {
			return err
		}
	}
	c, err := s.store.Conversation(id)
	if err != nil {
		return err
	}
	if state.File != "" && c.SessionFile != state.File {
		if _, err = s.store.BindSession(id, state.File); err != nil {
			return err
		}
	}
	if state.Total > 0 && !c.HasMessages {
		if _, err = s.store.UpdateConversation(id, func(c *Conversation) error { c.HasMessages = true; return nil }); err != nil {
			return err
		}
	}
	draft, err := s.store.ReadDraft(id)
	if err != nil {
		return err
	}
	if draft.PendingID != "" && slices.Contains(state.SavedInputs, draft.PendingID) {
		return s.store.ConfirmInput(id, draft.PendingID)
	}
	return nil
}
func (s *Service) event(id string, p *liveSession, raw json.RawMessage) {
	var value struct {
		Type, ID, Status, Error string
		Request                 json.RawMessage
		State                   json.RawMessage
		Paused                  bool
		BackgroundRunning       bool
		EventType, ToolCallID   string
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	var persistence error
	if value.Type == "input_saved" {
		_, persistence = s.store.UpdateConversation(id, func(c *Conversation) error { c.HasMessages = true; return nil })
		if persistence == nil {
			persistence = s.store.ConfirmInput(id, value.ID)
		}
	}
	if value.Type == "settled" || value.Type == "resumed" {
		persistence = s.remember(id, value.State)
	}
	p.mu.Lock()
	switch value.Type {
	case "execution_started":
		p.running = true
		p.status = "running"
		p.failure = ""
	case "durable_state":
		p.backgroundRunning = value.BackgroundRunning
		p.sessionPaused = value.Paused
		if value.Paused {
			p.running = false
			p.status = "paused"
		}
	case "resumed":
		var state struct{ Streaming, Paused, BackgroundRunning bool }
		if json.Unmarshal(value.State, &state) == nil {
			p.backgroundRunning = state.BackgroundRunning
			p.sessionPaused = state.Paused
			p.running = state.Streaming
			if state.Streaming {
				p.status, p.failure = "running", ""
			} else if state.Paused {
				p.status = "paused"
			} else if p.status == "paused" {
				p.status = "idle"
			}
			// Reopening an idle runtime must not erase its previous outcome.
		}
	case "approval":
		var request struct{ ID string }
		if json.Unmarshal(value.Request, &request) == nil {
			p.approvals[request.ID] = value.Request
		}
	case "approval_cancelled":
		delete(p.approvals, value.ID)
	case "settled":
		var tagged map[string]json.RawMessage
		if json.Unmarshal(raw, &tagged) == nil {
			if len(tagged["requestId"]) == 0 {
				tagged["requestId"], _ = json.Marshal(p.activeRequest)
			}
			if encoded, err := json.Marshal(tagged); err == nil {
				raw = encoded
			}
		}
		p.running = false
		p.status = value.Status
		p.failure = value.Error
		p.approvals = map[string]json.RawMessage{}
		p.activity.tools = nil
	case "pi_event":
		p.activity.trackTool(value.EventType, value.ToolCallID)
	}
	if persistence != nil {
		p.failure = "保存 Chat 状态失败：" + persistence.Error()
	}
	p.sequence++
	p.events = append(p.events, Event{p.sequence, raw})
	p.bytes += len(raw)
	// Slow or closed UIs can resynchronize from Pi's persisted state. They never
	// back-pressure stdout or make the agent accumulate unbounded event memory.
	for len(p.events) > 1 && (p.bytes > 2<<20 || len(p.events) > 512) {
		p.bytes -= len(p.events[0].Data)
		p.events = p.events[1:]
	}
	idle := !p.running && !p.backgroundRunning && len(p.owners) == 0 && p.executionOwners == 0
	p.notifyLocked()
	p.mu.Unlock()
	if (value.Type == "settled" || value.Type == "input_settled") && s.options.OnTurnSettled != nil {
		s.options.OnTurnSettled(id)
	}
	if idle {
		go s.closeIdle(id, p)
	}
}
func (s *Service) Poll(id string, after uint64) (EventPage, error) {
	p, err := s.session(id)
	if err != nil {
		return EventPage{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	page := EventPage{Events: []Event{}, Cursor: after, Running: p.running, Status: p.status, Error: p.failure, Approvals: []json.RawMessage{}}
	for _, request := range p.approvals {
		page.Approvals = append(page.Approvals, request)
	}
	page.Reset = after > p.sequence || len(p.events) > 0 && after+1 < p.events[0].Sequence
	if page.Reset {
		page.Cursor = p.sequence
		return page, nil
	}
	bytes := 0
	for _, event := range p.events {
		if event.Sequence <= after {
			continue
		}
		if len(page.Events) > 0 && bytes+len(event.Data) > 700<<10 {
			page.More = true
			break
		}
		page.Events = append(page.Events, event)
		page.Cursor = event.Sequence
		bytes += len(event.Data)
	}
	return page, nil
}
func (s *Service) Prompt(ctx context.Context, id, requestID, text string, attachmentPaths ...[]string) (json.RawMessage, error) {
	return s.submitInput(ctx, id, requestID, text, "queue", false, nil, attachmentPaths...)
}

// Steer shares validation, draft protection and durable receipts with Prompt.
// It does not abort the current turn; Pi delivers at its next input boundary.
func (s *Service) Steer(ctx context.Context, id, requestID, text string, attachmentPaths ...[]string) (json.RawMessage, error) {
	return s.submitInput(ctx, id, requestID, text, "steer", false, nil, attachmentPaths...)
}

// DispatchInput uses the same native queue and permission checks as Prompt,
// but a background delivery must never replace the user's unsent draft.
func (s *Service) DispatchInput(ctx context.Context, id, requestID, text string, attachmentPaths ...[]string) (json.RawMessage, error) {
	return s.submitInput(ctx, id, requestID, text, "queue", true, nil, attachmentPaths...)
}

// NativeInputPresentation separates host-authored context from what the user
// wrote. Only private dispatch adapters supply provenance; public prompts do not.
type NativeInputPresentation struct {
	Text            *string         `json:"text,omitempty"`
	Hidden          bool            `json:"hidden,omitempty"`
	ReviewRequest   json.RawMessage `json:"reviewRequest,omitempty"`
	OriginSessionID string          `json:"originSessionId,omitempty"`
	// ToolsDisabled runs this one input with no tools (Cindy's invitation
	// welcome). The worker restores the session's tools when the turn settles.
	ToolsDisabled bool `json:"toolsDisabled,omitempty"`
	// ReplyQuote is the owner's quote of an earlier message in a companion
	// chat (Cindy ChatReplyQuote). It is saved with the input for display only.
	ReplyQuote json.RawMessage `json:"replyQuote,omitempty"`
	// AgentReferences are the owner's @teammate references in that text (Cindy
	// agentReferences), kept without host snapshots for display.
	AgentReferences json.RawMessage `json:"agentReferences,omitempty"`
}

func (s *Service) DispatchPresentedInput(ctx context.Context, id, requestID, text, mode string, presentation NativeInputPresentation, attachmentPaths ...[]string) (json.RawMessage, error) {
	if mode != "queue" && mode != "steer" {
		return nil, errors.New("输入交付方式无效")
	}
	return s.submitInput(ctx, id, requestID, text, mode, true, &presentation, attachmentPaths...)
}

// PromptPresented submits an owner's prompt whose model text differs from what
// the owner wrote (a companion reply-quote note before it). The draft and the
// transcript keep the owner's own text; receipts work exactly as for Prompt.
func (s *Service) PromptPresented(ctx context.Context, id, requestID, text, mode string, presentation NativeInputPresentation, attachmentPaths []string) (json.RawMessage, error) {
	if mode != "queue" && mode != "steer" {
		return nil, errors.New("输入交付方式无效")
	}
	return s.submitInput(ctx, id, requestID, text, mode, false, &presentation, attachmentPaths)
}

func (s *Service) submitInput(ctx context.Context, id, requestID, text, deliveryMode string, native bool, presentation *NativeInputPresentation, attachmentPaths ...[]string) (json.RawMessage, error) {
	var paths []string
	if len(attachmentPaths) > 1 {
		return nil, errors.New("附件参数无效")
	}
	if len(attachmentPaths) == 1 {
		paths = attachmentPaths[0]
	}
	attachments, err := s.store.AttachmentDetails(id, paths)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" && len(paths) == 0 || len(text) > 100000 || requestID == "" || len(requestID) > 128 {
		return nil, errors.New("消息内容或标识无效")
	}
	p, err := s.session(id)
	if err != nil {
		return nil, err
	}
	if err = p.lock(ctx); err != nil {
		return nil, err
	}
	defer p.unlock()
	if p.runner == nil {
		return nil, errors.New("对话未就绪")
	}
	if !p.initialized {
		return nil, errors.New(p.modelError)
	}
	// A retry after a desktop/worker restart must consult Pi's persisted input
	// receipt before sending. Timeouts cannot silently execute a prompt twice.
	raw, err := p.runner.Call(ctx, "input_status", map[string]any{"id": requestID, "text": text, "attachments": paths, "deliveryMode": deliveryMode})
	if err != nil {
		return nil, err
	}
	var state struct {
		Saved     bool            `json:"saved"`
		Queued    bool            `json:"queued"`
		Execution json.RawMessage `json:"execution"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.Saved {
		result, err := json.Marshal(map[string]any{"accepted": true, "replayed": true, "queued": state.Queued, "execution": state.Execution})
		if err != nil {
			return nil, err
		}
		if native {
			return result, nil
		}
		return result, s.store.ConfirmInput(id, requestID)
	}
	p.mu.Lock()
	running, paused := p.running, p.sessionPaused
	p.mu.Unlock()
	if paused {
		return nil, errors.New("对话已暂停，请先恢复；草稿已保留")
	}
	// Both modes use Pi's durable inbox. Only the scheduling boundary differs;
	// the same receipt prevents ambiguous retries becoming a second input.
	if !native {
		draft := text
		if presentation != nil && presentation.Text != nil {
			draft = *presentation.Text
		}
		if err = s.store.SaveDraft(id, Draft{Text: draft, PendingID: requestID, PendingDelivery: deliveryMode, Attachments: attachments}); err != nil {
			return nil, err
		}
	}
	p.mu.Lock()
	p.running = true
	if !running {
		p.activeRequest = requestID
	}
	p.status = "running"
	p.failure = ""
	p.notifyLocked()
	p.mu.Unlock()
	raw, err = p.runner.CallOnce(ctx, requestID, "prompt", map[string]any{"text": text, "attachments": paths, "deliveryMode": deliveryMode, "trackExecution": native, "allowHeld": native, "presentation": presentation})
	if err != nil {
		// An ambiguous acknowledgement remains pending for state reconciliation.
		// Marking it idle here would allow a second prompt into a running agent.
		p.mu.Lock()
		var rejection *CommandRejection
		if !running && errors.As(err, &rejection) {
			p.running = false
			p.status = "failed"
		}
		p.failure = err.Error()
		p.notifyLocked()
		p.mu.Unlock()
	}
	// Cindy separates accepted queued input from the next unsent composer draft.
	// Pi persists winone.queued before acknowledging, so we can clear this exact
	// pending receipt now instead of waiting for consumption/history refresh.
	// ConfirmInput never clears a newer draft or a different pending receipt.
	if err == nil {
		var result struct {
			Accepted bool `json:"accepted"`
			Queued   bool `json:"queued"`
		}
		if err = json.Unmarshal(raw, &result); err == nil && result.Accepted {
			if native && result.Queued && !running {
				p.mu.Lock()
				p.running = false
				p.status = "idle"
				p.notifyLocked()
				p.mu.Unlock()
			}
			err = s.store.ConfirmInput(id, requestID)
		}
	}
	return raw, err
}

// WithIdleConversation shares the prompt gate. UI file recovery cannot race
// a new turn or change files while the agent is working on them.
func (s *Service) WithIdleConversation(ctx context.Context, id string, apply func() error) error {
	p, err := s.session(id)
	if err != nil {
		return err
	}
	if err = p.lock(ctx); err != nil {
		return err
	}
	defer p.unlock()
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	if running {
		return ErrBusy
	}
	return apply()
}

func (s *Service) Command(ctx context.Context, id, method string, params any) (json.RawMessage, error) {
	return s.CommandPrepared(ctx, id, method, params, nil)
}

// Preparation runs under the same gate as the command. Editing can restore a
// captured turn before replacing its input without admitting a concurrent prompt.
func (s *Service) CommandPrepared(ctx context.Context, id, method string, params any, prepare func() error) (json.RawMessage, error) {
	switch method {
	case "state", "resources", "permission", "question", "abort", "context.reset", "pause", "resume", "set_model", "set_permission", "rename", "compact", "retry", "thinking", "fast", "control", "queue", "timeline.append", "timeline.voice", "input.result", "input.source", "abort.input", "execution.inspect", "task.hold":
	default:
		return nil, errors.New("不支持的 Chat 操作")
	}
	p, err := s.session(id)
	if err != nil {
		return nil, err
	}
	if err = p.lock(ctx); err != nil {
		return nil, err
	}
	defer p.unlock()
	if p.runner == nil {
		return nil, errors.New("对话未就绪")
	}
	if method == "state" {
		c, err := s.store.Conversation(id)
		if err != nil {
			return nil, err
		}
		raw, err := s.readLocked(ctx, c, p, params)
		if err == nil {
			err = s.remember(id, raw)
		}
		return raw, err
	}
	if !p.initialized {
		return nil, errors.New(p.modelError)
	}
	p.mu.Lock()
	busy, previousStatus := p.running, p.status
	p.mu.Unlock()
	if busy && (method == "set_model" || method == "set_permission" || method == "compact" || method == "retry" || method == "thinking") {
		return nil, errors.New("请先停止当前任务")
	}
	var requestID string
	if method == "retry" {
		if input, ok := params.(map[string]any); ok {
			requestID, _ = input["requestId"].(string)
			if len(requestID) > 128 {
				return nil, errors.New("消息标识无效")
			}
		}
	}
	if method == "compact" || method == "retry" {
		if prepare != nil {
			if err := prepare(); err != nil {
				return nil, err
			}
		}
		p.mu.Lock()
		p.running = true
		p.status = "running"
		if requestID != "" {
			p.activeRequest = requestID
		}
		p.failure = ""
		p.mu.Unlock()
	}
	var raw json.RawMessage
	if requestID != "" {
		// Preserve the UI receipt through retry/edit just as for ordinary sends.
		raw, err = p.runner.CallOnce(ctx, requestID, method, params)
	} else {
		raw, err = p.runner.Call(ctx, method, params)
	}
	if err == nil && method == "retry" {
		// CallOnce can return an earlier accepted acknowledgement after its
		// turn already completed. Reconcile lifecycle from the Worker, never
		// send this snapshot to the UI over its ordered branch/delta stream.
		var state json.RawMessage
		state, err = p.runner.Call(ctx, "state", nil)
		if err == nil {
			var snapshot struct{ Streaming bool }
			err = json.Unmarshal(state, &snapshot)
			if err == nil {
				p.mu.Lock()
				p.running = snapshot.Streaming
				if !snapshot.Streaming && p.status == "running" {
					p.status = previousStatus
				}
				p.notifyLocked()
				p.mu.Unlock()
			}
		}
	}
	if err != nil && method == "retry" {
		p.mu.Lock()
		var rejection *CommandRejection
		if errors.As(err, &rejection) {
			p.running = false
			p.status = "failed"
		}
		p.failure = err.Error()
		p.notifyLocked()
		p.mu.Unlock()
	}
	if err == nil {
		if method == "execution.inspect" || method == "task.hold" {
			var execution struct {
				Running, Paused, Held, QueuePaused bool
				Pending                            []json.RawMessage
			}
			if err = json.Unmarshal(raw, &execution); err == nil && (execution.Running || !execution.Paused && !execution.Held && !execution.QueuePaused && len(execution.Pending) > 0) {
				// A released durable queue can start without Host.Prompt. Publish
				// that reservation before releasing the native execution lease;
				// otherwise idle cleanup can close the just-resumed worker.
				p.mu.Lock()
				p.running = true
				p.status = "running"
				p.notifyLocked()
				p.mu.Unlock()
			}
		}
		if method == "state" || method == "abort" || method == "context.reset" {
			err = s.remember(id, raw)
		}
		if method == "abort" || method == "context.reset" {
			p.mu.Lock()
			p.running = false
			p.status = "stopped"
			if method == "context.reset" {
				p.status, p.failure, p.activeRequest = "idle", "", ""
			}
			p.approvals = map[string]json.RawMessage{}
			p.notifyLocked()
			p.mu.Unlock()
		}
		if method == "permission" || method == "question" {
			data, _ := json.Marshal(params)
			var value struct{ ID string }
			json.Unmarshal(data, &value)
			p.mu.Lock()
			delete(p.approvals, value.ID)
			p.mu.Unlock()
		}
	}
	return raw, err
}

// Replace the idle runtime before committing model/permission preferences. A
// failed save keeps the old metadata; the next open can recreate that runtime.
func (s *Service) ConfigureConversation(ctx context.Context, id string, apply func(*Conversation) error) (Conversation, error) {
	return s.configureConversation(ctx, id, func() (Conversation, error) { return s.store.UpdateConversation(id, apply) })
}

func (s *Service) configureConversation(ctx context.Context, id string, commit func() (Conversation, error)) (Conversation, error) {
	if _, err := s.store.Conversation(id); err != nil {
		return Conversation{}, err
	}
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		return Conversation{}, errors.New("Chat 正在退出")
	}
	p := s.sessions[id]
	if p == nil {
		p = s.newSessionLocked(id)
	}
	s.mu.Unlock()
	if err := p.lock(ctx); err != nil {
		return Conversation{}, err
	}
	defer p.unlock()
	p.mu.Lock()
	busy := p.running
	p.mu.Unlock()
	if busy {
		return Conversation{}, errors.New("请先停止当前任务")
	}
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	if p.runner != nil {
		if err := p.runner.Close(ctx); err != nil {
			select {
			case <-p.runner.Done():
				s.retire(id, p)
			default:
			}
			return Conversation{}, err
		}
	}
	value, err := commit()
	s.retire(id, p)
	return value, err
}

// Pi creates and serializes the branch. WinOne only allocates its destination
// and records the returned session file; the source history is never rewritten.
func (s *Service) Fork(ctx context.Context, id, entry, title string) (Conversation, error) {
	p, err := s.session(id)
	if err != nil {
		return Conversation{}, err
	}
	if err = p.lock(ctx); err != nil {
		return Conversation{}, err
	}
	defer p.unlock()
	p.mu.Lock()
	busy := p.running
	p.mu.Unlock()
	if busy {
		return Conversation{}, errors.New("请先停止当前任务")
	}
	source, err := s.store.Conversation(id)
	if err != nil {
		return Conversation{}, err
	}
	if source.SessionFile == "" || !source.HasMessages {
		return Conversation{}, errors.New("对话有消息后才能创建分支")
	}
	if title == "" {
		title = source.Title + " · Fork"
	}
	branch, err := s.store.NewConversation(source.ProjectID, source.Directory, title, source.Model)
	if err != nil {
		return Conversation{}, err
	}
	raw, err := p.runner.Call(ctx, "fork", map[string]any{"file": source.SessionFile, "cwd": source.Directory, "directory": branch.SessionDirectory, "entry": entry, "name": title})
	if err == nil {
		err = s.remember(branch.ID, raw)
	}
	if err != nil {
		_, _ = s.store.UpdateConversation(branch.ID, func(c *Conversation) error { c.Archived = true; return nil })
		return Conversation{}, err
	}
	return s.store.UpdateConversation(branch.ID, func(c *Conversation) error { c.Permission = source.Permission; return nil })
}

// Detach preserves running work, then releases its process when it settles.
func (s *Service) Detach(owner uint32) {
	s.mu.Lock()
	sessions := make(map[string]*liveSession, len(s.sessions))
	for id, p := range s.sessions {
		sessions[id] = p
	}
	s.mu.Unlock()
	for id, p := range sessions {
		p.mu.Lock()
		delete(p.owners, owner)
		p.mu.Unlock()
		go s.closeIdle(id, p)
	}
}
func (s *Service) Release(id string, owner uint32) {
	p, err := s.session(id)
	if err != nil {
		return
	}
	p.mu.Lock()
	if owner == 0 {
		if p.executionOwners > 0 {
			p.executionOwners--
		}
	} else {
		delete(p.owners, owner)
	}
	p.mu.Unlock()
	go s.closeIdle(id, p)
}
func (s *Service) closeIdle(id string, p *liveSession) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if p.lock(ctx) != nil {
		return
	}
	defer p.unlock()
	p.mu.Lock()
	idle := !p.running && !p.backgroundRunning && len(p.owners) == 0 && p.executionOwners == 0 && !p.closed
	if idle {
		// Done may fire before retire. Mark an expected close first so it
		// cannot overwrite a stopped/completed outcome with a pipe failure.
		p.closed = true
	}
	p.mu.Unlock()
	if !idle {
		return
	}
	if p.runner != nil {
		if err := p.runner.Close(ctx); err != nil {
			p.mu.Lock()
			p.failure = err.Error()
			p.mu.Unlock()
		}
	}
	if p.runner != nil {
		select {
		case <-p.runner.Done():
		default:
			p.mu.Lock()
			p.closed = false // A failed close remains retryable.
			p.mu.Unlock()
			return
		}
	}
	s.retire(id, p)
}
func (s *Service) retire(id string, p *liveSession) {
	p.mu.Lock()
	p.closed = true
	outcome := sessionOutcome{status: p.status, failure: p.failure, paused: p.sessionPaused, activity: p.activity.retired()}
	p.notifyLocked()
	p.mu.Unlock()
	if p.release != nil {
		p.release()
		p.release = nil
	}
	s.mu.Lock()
	if s.sessions[id] == p {
		if s.outcomes == nil {
			s.outcomes = make(map[string]sessionOutcome)
		}
		s.outcomes[id] = outcome
		delete(s.sessions, id)
	}
	s.mu.Unlock()
}
func (s *Service) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions) > 0 || s.archiveRunner != nil
}

// CloseSession is the same owned-worker shutdown used by Stop, scoped to one
// conversation. It preserves the transcript and draft and can be reopened.
func (s *Service) CloseSession(ctx context.Context, id string) error {
	s.mu.Lock()
	p := s.sessions[id]
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	if err := p.lock(ctx); err != nil {
		return err
	}
	defer p.unlock()
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	var err error
	if p.runner != nil {
		err = p.runner.Close(ctx)
		select {
		case <-p.runner.Done():
		default:
			return err
		}
	}
	s.retire(id, p)
	return err
}

// Stop is a reversible preparation step. A later module may refuse to exit;
// Resume then allows reopening saved conversations without resurrecting tools.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.paused = true
	sessions := make(map[string]*liveSession, len(s.sessions))
	for id, p := range s.sessions {
		sessions[id] = p
	}
	archive := s.archiveRunner
	s.mu.Unlock()
	count := len(sessions)
	if archive != nil {
		count++
	}
	results := make(chan error, count)
	if archive != nil {
		go func() {
			err := archive.Close(ctx)
			s.mu.Lock()
			if s.archiveRunner == archive {
				s.archiveRunner = nil
			}
			s.mu.Unlock()
			results <- err
		}()
	}
	for id := range sessions {
		go func(id string) { results <- s.CloseSession(ctx, id) }(id)
	}
	var result error
	for range count {
		if err := <-results; err != nil {
			result = errors.Join(result, fmt.Errorf("停止 Chat：%w", err))
		}
	}
	return result
}
func (s *Service) Resume() { s.mu.Lock(); s.paused = false; s.mu.Unlock() }

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/chat"
	"winone/internal/processidentity"
)

// The OneBot process is lazy and owned by Chat's Windows Job lifecycle. Merely
// starting WinOne does not create a Bot Home, open IM accounts or start polling.
type chatOneBot struct {
	control           *chatControl
	startMu           sync.Mutex
	runner            *chat.Runner
	stopped           bool
	sessionMu         sync.Mutex
	sessions          *chatControl
	refreshMu         sync.Mutex
	refresh           map[string]uint64
	restartContexts   map[string]oneBotContext
	deletionContext   *oneBotDeletionContext
	recoveryCancel    context.CancelFunc
	recoveryWG        sync.WaitGroup
	learningChanged   chan struct{}
	imBridge          *chat.PrivateBridge
	imDriver          *chatAutomations
	grantMu           sync.Mutex
	grants            map[string]oneBotInputGrant
	grantsLoaded      bool
	eventMu           sync.Mutex
	eventSequence     uint64
	events            []oneBotEvent
	eventsChanged     chan struct{}
	runtimeCandidates map[string]oneBotRuntimeCandidate
	protectedTurns    sync.Map // session/input -> memory-only Pi worker; never transcript storage
	// Teammate follow-up: conversations whose activity changed since the last
	// Worker push, and the tray reminder (replaced in tests).
	activityMu        sync.Mutex
	activityPending   map[string]bool
	activityScheduled bool
	activitySend      sync.Mutex
	remindNotify      func(title, text string) error
}

func newChatOneBot(control *chatControl) *chatOneBot {
	bot := &chatOneBot{control: control, refresh: map[string]uint64{}, grants: map[string]oneBotInputGrant{}, eventsChanged: make(chan struct{}), learningChanged: make(chan struct{}, 1), runtimeCandidates: map[string]oneBotRuntimeCandidate{}}
	control.onebot = bot
	return bot
}

// Keep the frontend/Host boundary explicit. init and ownership controls remain
// private; receipts are not exposed as executable todo actions in the UI.
func oneBotMethod(method string) (allowed, readOnly bool) {
	if strings.HasPrefix(method, "voice.") {
		switch method {
		case "voice.models", "voice.voices", "voice.preference", "voice.readPreview", "voice.readSpeech", "voice.input.data", "voice.input.history", "voice.input.historyForRefinement":
			return true, true
		}
		// The Worker exposes a closed voice method table; private resource/credential
		// endpoints are hyphenated bridge routes and never use this desktop prefix.
		return true, false
	}
	if strings.HasPrefix(method, "chat.") {
		switch strings.TrimPrefix(method, "chat.") {
		case "catalog", "activity", "poll", "state", "open", "release", "detach", "draft.get", "resources", "files", "usage", "file.resolve", "file.stat", "file.preview", "files.list", "files.index", "files.search", "files.cancel", "media.source", "review.turn", "review.snapshot", "review.diff", "review.diff.preview", "review.file.preview", "review.metadata", "review.branches", "review.commits", "terminal.poll", "terminal.resize":
			return true, true
		default:
			return true, false
		}
	}
	switch method {
	case "wechat.state", "wechat.settings":
		return true, true
	case "wechat.authorize", "wechat.cancelAuthorization", "wechat.unbind", "wechat.setCompanion", "wechat.setWorkingDirectory", "wechat.resetWorkingDirectory":
		return true, false
	case "market.list", "market.status", "import.sources", "import.preview", "import.status":
		return true, true
	case "market.invite", "import.start", "bots.generateDraft", "bots.generateAvatar":
		return true, false
	case "groups.list", "groups.get", "groups.files.describe", "groups.files.read", "groups.files.classify", "groups.files.resolvePath", "groups.files.resolvePathBatch":
		return true, true
	case "groups.create", "groups.update", "groups.setMembers", "groups.delete", "groups.send", "groups.continue", "groups.stop",
		"groups.plan.start", "groups.plan.dismiss", "groups.plan.continue", "groups.plan.retry", "groups.plan.edit", "groups.attachments.stage", "groups.attachments.image", "groups.files.begin", "groups.files.chunk", "groups.files.finish", "groups.files.image", "groups.files.discard", "groups.files.open", "groups.files.reveal", "groups.files.remoteImage", "groups.files.draftUrls":
		return true, false
	case "routines.list", "routines.history", "routines.sources", "routines.nightly.get":
		return true, true
	case "routines.save", "routines.remove", "routines.runNow", "routines.nightly.setEnabled", "routines.nightly.setFrequency", "routines.nightly.runNow", "routines.nightly.revert", "routines.nightly.candidate":
		return true, false
	case "tasks.pr.refs", "tasks.pr.statuses", "tasks.list", "tasks.get", "capabilities.get", "events", "projects.list", "history.search", "im.list", "profiles.list", "profiles.get", "profiles.versions", "profile.content.read",
		"imConnection.list", "imConnection.listGroups", "imConnection.listLanes", "imConnection.listAdoptable",
		"bots.list", "bots.history", "avatar.source", "media.read", "modelChain.get", "dmThread.list", "workbench.get", "workbench.followScopes", "workbench.readTask", "directMessage.get",
		"memory.list", "memory.read", "memory.index", "memory.search", "messages.learning",
		"skills.list", "skills.query", "skills.read", "skills.readSource", "skills.mounts", "todos.read",
		"autonomy.glance", "autonomy.settings":
		return true, true
	case "tasks.stop", "tasks.message", "tasks.trace", "projects.set", "lifecycle.action", "im.add", "im.remove", "im.reconnect", "im.online", "im.offline", "im.groups.setMode", "session.open", "profiles.create", "profiles.update", "profile.content.write", "memory.update", "memory.delete",
		"imConnection.add", "imConnection.remove", "imConnection.setGroupActivation", "imConnection.resetLane", "imConnection.archiveLane", "imConnection.adopt",
		"bots.createCanonicalSession", "bots.chooseAvatar", "modelChain.set", "modelChain.reset", "dmThread.ensure", "todos.act",
		"memory.write", "memory.consolidate", "memory.importDocument", "skills.save", "skills.saveSource", "skills.delete", "todos.patch",
		"autonomy.setSettings", "autonomy.resetSettings", "todos.dailyCheck":
		return true, false
	default:
		return false, false
	}
}

func (b *chatOneBot) event(raw json.RawMessage) {
	var message struct {
		Type  string `json:"type"`
		Event struct {
			Type      string          `json:"type"`
			Level     string          `json:"level"`
			Scope     string          `json:"scope"`
			Message   string          `json:"message"`
			SessionID string          `json:"sessionId"`
			BotID     string          `json:"botId"`
			Payload   json.RawMessage `json:"payload"`
		} `json:"event"`
	}
	if json.Unmarshal(raw, &message) != nil || message.Type != "onebot_event" {
		return
	}
	if message.Event.Type == "runtime-refresh-requested" && message.Event.SessionID != "" {
		b.refreshMu.Lock()
		b.refresh[message.Event.SessionID]++
		b.refreshMu.Unlock()
		return
	}
	if message.Event.Type != "diagnostic" {
		switch message.Event.Type {
		case "task-changed", "profile-changed", "lifecycle-changed", "workbench-changed", "capabilities-changed":
			if message.Event.BotID != "" {
				// Push invalidation only; memory text, credentials and arbitrary
				// Worker diagnostic detail never become desktop event payloads.
				b.publishEvent(message.Event.Type, message.Event.BotID, oneBotEventPayload(message.Event.Payload))
			}
		case "autonomy-changed", "routines-changed", "im-changed", "group-changed", "direct-message-changed", "message-created", "message-meta-changed", "voice", "voice-input", "voice-data", "wechat-state", "wechat-settings":
			// The upstream push bodies the teammate pages read (ids, states and
			// display lists); never credentials, memory text or diagnostics.
			b.publishEvent(message.Event.Type, message.Event.BotID, oneBotEventPayload(message.Event.Payload))
		}
		return
	}
	// Keep diagnostic summaries in WinOne's shared bounded log. Arbitrary detail
	// arrays may contain memory/Skill bodies and are deliberately not serialized.
	if err := recordHostEvent(b.control.directory, "onebot:"+message.Event.Scope+":"+message.Event.Level, errors.New(message.Event.Message)); err != nil {
		fmt.Fprintln(os.Stderr, "OneBot diagnostic log:", err)
	}
}

func (b *chatOneBot) ensure(ctx context.Context) (*chat.Runner, error) {
	b.startMu.Lock()
	defer b.startMu.Unlock()
	if b.stopped {
		return nil, errors.New("OneBot 正在退出")
	}
	if b.runner != nil {
		select {
		case <-b.runner.Done():
			if b.recoveryCancel != nil {
				b.recoveryCancel()
			}
			b.runner = nil
			if b.imBridge != nil {
				b.imBridge.Close()
				b.imBridge = nil
			}
		default:
			return b.runner, nil
		}
	}
	directory := filepath.Join(b.control.directory, "onebot")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	base := filepath.Dir(b.control.desktop)
	if b.sessions == nil {
		store, openErr := chat.Open(filepath.Join(directory, "chat"))
		if openErr != nil {
			return nil, openErr
		}
		inner := &chatControl{ctx: b.control.ctx, directory: directory, desktop: b.control.desktop, language: b.control.language, ai: b.control.ai, store: store, owners: map[uint32]*processidentity.Process{}}
		inner.guard = b.control.guard
		inner.prepareInput = b.prepareOwnerInput
		inner.terminals = chat.NewTerminalManager(b.control.ctx)
		inner.engine = chat.NewService(b.control.ctx, store, chat.ServiceOptions{Bun: filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), Worker: filepath.Join(base, "shared", "chat", "worker.js"), Configure: b.configureSession, ResourceConfig: b.inspectSessionResources, OnTurnSettled: func(string) {
			select {
			case b.learningChanged <- struct{}{}:
			default:
			}
		}, OnActivity: b.noteActivity})
		b.sessions = inner
		b.imDriver = newChatAutomations(inner)
	}
	masterKey, err := oneBotMasterKey(directory)
	if err != nil {
		return nil, err
	}
	bridge, err := chat.NewPrivateBridge(b.control.ctx, http.HandlerFunc(b.serveIM))
	if err != nil {
		return nil, err
	}
	b.imBridge = bridge
	// The reused command runner needs this authenticated callback bridge. The
	// helper has no scheduler process of its own; OneBot retains bridge ownership.
	b.imDriver.bridge = bridge
	runner, err := chat.StartRunner(b.control.ctx, filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), filepath.Join(base, "shared", "chat", "onebot.js"), directory, b.event)
	if err != nil {
		bridge.Close()
		b.imBridge = nil
		return nil, err
	}
	if _, err = runner.Call(ctx, "init", map[string]any{"directory": directory, "masterKey": masterKey, "dispatchBaseUrl": bridge.BaseURL, "dispatchKey": bridge.Key, "language": b.control.language()}); err != nil {
		// The runner uses a Job kill-on-close, including an init failure or deadline.
		_ = runner.Close(ctx)
		bridge.Close()
		b.imBridge = nil
		return nil, err
	}
	b.runner = runner
	// Recovery cannot run inside init: a saved inbound turn may call this Host
	// bridge, which resolves its profile through the already-published runner.
	resume, cancel := context.WithCancel(b.control.ctx)
	b.recoveryCancel = cancel
	b.recoveryWG.Add(1)
	go func() {
		defer b.recoveryWG.Done()
		defer cancel()
		startup, finishStartup := context.WithTimeout(resume, 30*time.Second)
		defer finishStartup()
		if err := b.recoverDeletedBots(startup, runner); err != nil {
			if resume.Err() == nil {
				_ = recordHostEvent(b.control.directory, "onebot:cleanup-recovery", err)
			}
		}
		if resume.Err() != nil {
			return
		}
		if _, err := runner.Call(startup, "tasks.recover", map[string]any{}); err != nil && resume.Err() == nil {
			_ = recordHostEvent(b.control.directory, "onebot:task-recovery", err)
		}
		if _, err := runner.Call(startup, "im.startSaved", nil); err != nil {
			if resume.Err() == nil {
				_ = recordHostEvent(b.control.directory, "onebot:im-recovery", err)
			}
		}
		finishStartup()
		// A single coalesced invalidation covers both UI and headless IM turns.
		// Startup also replays replies accepted before a crash; receipts dedupe.
		for {
			scan, finishScan := context.WithTimeout(resume, 30*time.Second)
			_, scanErr := runner.Call(scan, "learning.scan", map[string]any{})
			finishScan()
			if scanErr != nil && resume.Err() == nil {
				_ = recordHostEvent(b.control.directory, "onebot:learning-recovery", scanErr)
			}
			select {
			case <-resume.Done():
				return
			case <-b.learningChanged:
			}
		}
	}()
	return runner, nil
}

func (b *chatOneBot) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method == "session.open" {
		var input struct {
			BotID string `json:"botId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		value, err := b.openSession(ctx, input.BotID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	}
	if allowed, _ := oneBotMethod(method); !allowed {
		return nil, errors.New("不支持的 OneBot 操作")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if method == "tasks.pr.refs" || method == "tasks.pr.statuses" {
		var input struct {
			ChildSessionID string `json:"childSessionId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		row, err := b.control.store.Conversation(input.ChildSessionID)
		if err != nil {
			return nil, err
		}
		if row.OneBotTaskID == "" {
			return nil, errors.New("会话不属于 OneBot 独立任务")
		}
		return runner.Call(ctx, method, map[string]string{"childSessionId": row.ID})
	}
	if method == "tasks.list" || method == "tasks.get" || method == "tasks.stop" || method == "tasks.message" || method == "tasks.trace" {
		return b.taskUICall(ctx, runner, method, params)
	}
	if method == "events" {
		return b.waitEvents(ctx, runner, params)
	}
	if method == "capabilities.get" {
		return b.capabilityCatalog(ctx, runner, params)
	}
	if method == "projects.list" || method == "projects.set" {
		return b.projectCall(ctx, runner, method, params)
	}
	if method == "history.search" {
		return b.searchHistory(ctx, runner, params)
	}
	switch method {
	case "directMessage.get":
		// Cindy's teammate conversation view; the thread lives in the task store.
		return runner.Call(ctx, "tasks.directMessage.get", params)
	case "voice.input.microphoneSettings":
		return marshalOneBot(openOneBotMicrophoneSettings())
	case "groups.files.open", "groups.files.reveal":
		return marshalOneBot(b.groupFileAction(ctx, method, params))
	case "bots.list":
		return b.listBots(ctx, runner, params)
	case "bots.createCanonicalSession":
		return marshalOneBot(b.createCanonicalSession(ctx, runner, params))
	case "bots.history":
		return marshalOneBot(b.botHistory(ctx, runner, params))
	case "dmThread.ensure":
		return marshalOneBot(b.ensureDmThread(ctx, runner, params))
	case "dmThread.list":
		return marshalOneBot(b.listDmThreads(ctx, runner, params))
	case "todos.act":
		return b.actTodo(ctx, runner, params)
	case "avatar.source":
		return marshalOneBot(b.avatarSource(params))
	case "media.read":
		return marshalOneBot(b.mediaRead(params))
	case "modelChain.get", "modelChain.set", "modelChain.reset":
		return marshalOneBot(b.modelChainSettings(method, params))
	case "bots.chooseAvatar":
		// The renderer supplies cropped bytes; the Worker validates and stores them.
		b.sessionMu.Lock()
		defer b.sessionMu.Unlock()
		raw, err := runner.Call(ctx, "bots.setAvatar", params)
		if err != nil {
			return nil, err
		}
		profile, err := b.projectProfiles(raw, false)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"canceled": false, "profile": profile})
	}
	switch method {
	case "profiles.get", "profiles.list", "profiles.create", "profiles.update":
		// Keep the Bot link set and Chat metadata together across open/delete.
		b.sessionMu.Lock()
		defer b.sessionMu.Unlock()
		raw, err := runner.Call(ctx, method, params)
		if err != nil {
			return nil, err
		}
		if err = b.flushRuntimeRefresh(ctx); err != nil {
			return nil, err
		}
		return b.projectProfiles(raw, method == "profiles.list")
	}
	if method == "lifecycle.action" {
		// Session creation and deletion share one admission lock. The callback
		// cannot query the worker while its lifecycle transaction is awaiting us.
		b.sessionMu.Lock()
		defer b.sessionMu.Unlock()
		var action struct {
			BotID           string `json:"botId"`
			Action          string `json:"action"`
			KeepTaskHistory bool   `json:"keepTaskHistory"`
		}
		if err := json.Unmarshal(params, &action); err != nil {
			return nil, err
		}
		if action.Action == "delete" {
			raw, err := runner.Call(ctx, "profiles.get", map[string]string{"botId": action.BotID})
			if err != nil {
				return nil, err
			}
			var profile struct {
				Sessions []struct {
					ID string `json:"id"`
				} `json:"sessions"`
			}
			if err := json.Unmarshal(raw, &profile); err != nil {
				return nil, err
			}
			snapshot := &oneBotDeletionContext{BotID: action.BotID, KeepTaskHistory: action.KeepTaskHistory}
			for _, session := range profile.Sessions {
				snapshot.SessionIDs = append(snapshot.SessionIDs, session.ID)
			}
			b.refreshMu.Lock()
			b.deletionContext = snapshot
			b.refreshMu.Unlock()
			defer func() { b.refreshMu.Lock(); b.deletionContext = nil; b.refreshMu.Unlock() }()
		}
		if action.Action == "restart" {
			snapshot, err := b.contextFor(ctx, map[string]any{"botId": action.BotID, "lifecycleRecovery": true})
			if err != nil {
				return nil, err
			}
			b.refreshMu.Lock()
			if b.restartContexts == nil {
				b.restartContexts = map[string]oneBotContext{}
			}
			b.restartContexts[snapshot.CanonicalSessionID] = snapshot
			b.refreshMu.Unlock()
			defer func() {
				b.refreshMu.Lock()
				delete(b.restartContexts, snapshot.CanonicalSessionID)
				b.refreshMu.Unlock()
			}()
		}
	}
	raw, err := runner.Call(ctx, method, params)
	if err == nil {
		err = b.flushRuntimeRefresh(ctx)
	}
	return raw, err
}

func marshalOneBot[T any](value T, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func (b *chatOneBot) close(ctx context.Context) error {
	b.startMu.Lock()
	b.stopped = true
	b.refreshMu.Lock()
	b.runtimeCandidates = map[string]oneBotRuntimeCandidate{}
	b.refreshMu.Unlock()
	if b.recoveryCancel != nil {
		b.recoveryCancel()
		b.recoveryCancel = nil
	}
	runner, sessions, imBridge := b.runner, b.sessions, b.imBridge
	b.runner = nil
	b.imBridge = nil
	b.startMu.Unlock()
	// No startup callback may recreate logs or touch the data directory after
	// close returns. Cancellation is paired with joining the owned goroutines.
	b.recoveryWG.Wait()
	var sessionError error
	b.protectedTurns.Range(func(_, value any) bool {
		sessionError = errors.Join(sessionError, value.(*chat.Runner).Close(ctx))
		return true
	})
	if sessions != nil {
		sessionError = errors.Join(sessionError, sessions.stop())
	}
	if imBridge != nil {
		defer imBridge.Close()
	}
	if runner == nil {
		return sessionError
	}
	return errors.Join(sessionError, runner.Close(ctx))
}
func (b *chatOneBot) ownsConversation(params json.RawMessage) bool {
	var value struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(params, &value) != nil || value.ID == "" {
		return false
	}
	return b.ownsSession(value.ID)
}

func (b *chatOneBot) active() bool {
	b.startMu.Lock()
	sessions := b.sessions
	b.startMu.Unlock()
	return sessions != nil && sessions.active()
}

func (b *chatOneBot) resume() {
	b.startMu.Lock()
	b.stopped = false
	if b.sessions != nil {
		b.sessions.resume()
	}
	b.startMu.Unlock()
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"fyne.io/systray"
	"winone/internal/chat"
)

// Cindy's teammate follow-up (bot-autonomy, b7e46bc) watches every local
// conversation. On WinOne the Worker runs the original runtime; this Host side
// supplies conversation activity and metadata from both Chat stores and shows
// its owner reminders in the tray. Transcripts never cross this boundary.

// activityPushDelay collapses a burst of phase changes (a tool starting and
// ending, an approval arriving) into one Worker call.
const activityPushDelay = 200 * time.Millisecond

// maximumAutonomySessionIDs bounds one metadata read; TaskWatch asks only for
// conversations whose activity it has seen.
const maximumAutonomySessionIDs = 1000

// noteActivity queues a conversation whose phase, approvals, turn or tool
// state changed. Only a running OneBot Worker hears it: activity never starts
// the Worker, so an owner without teammates pays for nothing.
func (b *chatOneBot) noteActivity(id string) {
	b.activityMu.Lock()
	defer b.activityMu.Unlock()
	if b.activityPending == nil {
		b.activityPending = map[string]bool{}
	}
	b.activityPending[id] = true
	if !b.activityScheduled {
		b.activityScheduled = true
		time.AfterFunc(activityPushDelay, b.flushActivity)
	}
}

// flushActivity sends the queued conversations' current snapshots. Sends are
// serialized so the Worker always applies a newer read after an older one.
func (b *chatOneBot) flushActivity() {
	b.activitySend.Lock()
	defer b.activitySend.Unlock()
	b.activityMu.Lock()
	ids := make([]string, 0, len(b.activityPending))
	for id := range b.activityPending {
		ids = append(ids, id)
	}
	b.activityPending, b.activityScheduled = nil, false
	b.activityMu.Unlock()
	b.startMu.Lock()
	runner, sessions, stopped := b.runner, b.sessions, b.stopped
	b.startMu.Unlock()
	if runner == nil || stopped || len(ids) == 0 {
		return
	}
	select {
	case <-runner.Done():
		return
	default:
	}
	snapshots := b.activitySnapshots(sessions, ids...)
	// A conversation neither engine knows any more (deleted, never opened) is
	// idle, which removes it from the watch.
	for _, id := range ids {
		if !slices.ContainsFunc(snapshots, func(value chat.ActivitySnapshot) bool { return value.SessionID == id }) {
			snapshots = append(snapshots, chat.ActivitySnapshot{SessionID: id, Phase: "idle"})
		}
	}
	ctx, cancel := context.WithTimeout(b.control.ctx, 10*time.Second)
	defer cancel()
	if _, err := runner.Call(ctx, "autonomy.activity", map[string]any{"snapshots": snapshots}); err != nil && ctx.Err() == nil {
		_ = recordHostEvent(b.control.directory, "onebot:autonomy-activity", err)
	}
}

// activitySnapshots reads ordinary Chat and the teammates' own conversations.
// Without ids it returns every conversation that is not idle.
func (b *chatOneBot) activitySnapshots(sessions *chatControl, ids ...string) []chat.ActivitySnapshot {
	snapshots := []chat.ActivitySnapshot{}
	for _, control := range []*chatControl{b.control, sessions} {
		if control != nil && control.engine != nil {
			snapshots = append(snapshots, control.engine.ActivitySnapshots(ids...)...)
		}
	}
	return snapshots
}

// autonomySessions answers Cindy's LocalSessionMeta read for the given ids:
// title, working directory and archive state, plus which store holds them.
func (b *chatOneBot) autonomySessions(raw json.RawMessage) (map[string]any, error) {
	var input struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if len(input.IDs) > maximumAutonomySessionIDs {
		return nil, errors.New("too many conversations")
	}
	type meta struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		WorkingDir string `json:"workingDir"`
		Archived   bool   `json:"archived"`
		Store      string `json:"store"`
	}
	stores := []struct {
		name  string
		store *chat.Store
	}{{"chat", b.control.store}}
	if b.sessions != nil {
		stores = append(stores, struct {
			name  string
			store *chat.Store
		}{"onebot", b.sessions.store})
	}
	rows := []meta{}
	for _, id := range input.IDs {
		for _, entry := range stores {
			if entry.store == nil {
				continue
			}
			if conversation, err := entry.store.Conversation(id); err == nil {
				rows = append(rows, meta{ID: conversation.ID, Title: conversation.Title, WorkingDir: conversation.Directory, Archived: conversation.Archived, Store: entry.name})
				break
			}
		}
	}
	return map[string]any{"sessions": rows}, nil
}

// remindOwner is Cindy's desktop reminder for an unattended task that waits
// for its owner (no teammate follows it). WinOne shows it in the tray.
func (b *chatOneBot) remindOwner(raw json.RawMessage) (map[string]bool, error) {
	var input struct {
		SessionID string `json:"sessionId"`
		Title     string `json:"title"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	heading, untitled := "WinOne · 任务在等你回复", "未命名对话"
	if b.control.language != nil && b.control.language() == "en-US" {
		heading, untitled = "WinOne · A task is waiting for you", "Untitled conversation"
	}
	text := input.Title
	if text == "" {
		text = untitled
	}
	if input.Detail != "" {
		text += "\n" + input.Detail
	}
	if len([]rune(text)) > 1000 {
		text = string([]rune(text)[:1000])
	}
	notify := b.remindNotify
	if notify == nil {
		notify = systray.ShowWarning
	}
	if err := notify(heading, text); err != nil {
		return nil, err
	}
	return map[string]bool{"shown": true}, nil
}

// savedRoutinesActive reports whether a teammate that is not paused has
// routines. The nightly learning routine exists for every active teammate,
// so this also covers follow-up: both need the Worker running after launch.
func (b *chatOneBot) savedRoutinesActive() (bool, error) {
	file := filepath.Join(b.control.directory, "onebot", "routines", "routines.json")
	info, err := os.Stat(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() > 64<<20 {
		return false, errors.New("OneBot 例行任务记录过大")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return false, err
	}
	var state struct {
		Routines []struct {
			BotID string `json:"botId"`
		} `json:"routines"`
		PausedBotIDs []string `json:"pausedBotIds"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return false, err
	}
	for _, routine := range state.Routines {
		if routine.BotID != "" && !slices.Contains(state.PausedBotIDs, routine.BotID) {
			return true, nil
		}
	}
	return false, nil
}

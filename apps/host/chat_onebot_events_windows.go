//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"

	"winone/internal/chat"
)

type oneBotEvent struct {
	sequence uint64
	data     json.RawMessage
}
type oneBotEventPage struct {
	Cursor uint64            `json:"cursor"`
	Events []json.RawMessage `json:"events"`
	Reset  bool              `json:"reset"`
}

// oneBotEventPayload keeps a bounded upstream push body; larger bodies are
// dropped and subscribers re-read through their ordinary method.
func oneBotEventPayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(raw) > 64<<10 || string(raw) == "null" {
		return nil
	}
	return raw
}

func (b *chatOneBot) publishEvent(kind, botID string, payload ...json.RawMessage) {
	event := map[string]any{"type": kind, "botId": botID}
	if len(payload) > 0 && payload[0] != nil {
		event["payload"] = payload[0]
	}
	data, _ := json.Marshal(event)
	b.eventMu.Lock()
	defer b.eventMu.Unlock()
	b.eventSequence++
	b.events = append(b.events, oneBotEvent{b.eventSequence, data})
	if len(b.events) > 256 {
		b.events = b.events[len(b.events)-256:]
	}
	close(b.eventsChanged)
	b.eventsChanged = make(chan struct{})
}

// Like the existing scheduler long poll, this consumes one native request only
// while a page subscribes. A capped/missing cursor explicitly requests reload;
// dropped invalidations must not leave another open settings page stale.
func (b *chatOneBot) waitEvents(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Cursor uint64 `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	for {
		b.eventMu.Lock()
		page := oneBotEventPage{Cursor: b.eventSequence, Events: []json.RawMessage{}, Reset: input.Cursor > b.eventSequence}
		if len(b.events) > 0 && input.Cursor < b.events[0].sequence-1 {
			page.Reset = true
		}
		for _, event := range b.events {
			if event.sequence > input.Cursor {
				page.Events = append(page.Events, event.data)
			}
		}
		changed := b.eventsChanged
		b.eventMu.Unlock()
		if page.Reset || len(page.Events) > 0 {
			return json.Marshal(page)
		}
		select {
		case <-ctx.Done():
			return json.Marshal(page)
		case <-changed:
		case <-runner.Done():
			return nil, errors.New("OneBot 进程已退出")
		}
	}
}

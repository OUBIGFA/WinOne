//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type deferredModelChange struct{ Params map[string]any }

// DeferModelConfiguration accepts the next route without changing a live turn.
// The existing durable queue hold fences future inputs; their receipts remain
// in the same Pi inbox and are released only after the native model is applied.
func (s *Service) DeferModelConfiguration(ctx context.Context, id string, params map[string]any, apply func(*Conversation) error) (Conversation, error) {
	p, err := s.session(id)
	if err != nil {
		return Conversation{}, err
	}
	if err = p.lock(ctx); err != nil {
		return Conversation{}, err
	}
	defer p.unlock()
	p.mu.Lock()
	busy, closed := p.running || p.backgroundRunning, p.closed
	p.mu.Unlock()
	if closed || p.runner == nil {
		return Conversation{}, errors.New("对话未就绪")
	}
	if !busy {
		return Conversation{}, ErrBusy
	}
	if _, err = p.runner.Call(ctx, "queue", map[string]any{"action": "modelHold", "locked": true}); err != nil {
		return Conversation{}, err
	}
	row, err := s.store.UpdateConversation(id, func(value *Conversation) error {
		if err := apply(value); err != nil {
			return err
		}
		value.ModelDeferred = true
		return nil
	})
	if err != nil {
		_, _ = p.runner.Call(ctx, "queue", map[string]any{"action": "modelHold", "locked": false})
		return row, err
	}
	p.mu.Lock()
	p.modelChange = &deferredModelChange{Params: params}
	p.mu.Unlock()
	go s.applyDeferredModel(id, p)
	return row, nil
}
func (s *Service) applyDeferredModel(id string, p *liveSession) {
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	if err := p.lock(ctx); err != nil {
		return
	}
	defer p.unlock()
	p.mu.Lock()
	change, busy, closed, paused := p.modelChange, p.running || p.backgroundRunning, p.closed, p.sessionPaused
	p.mu.Unlock()
	if change == nil || busy || closed || paused || p.runner == nil {
		return
	}
	fail := func(err error) {
		p.mu.Lock()
		p.failure = "应用新的任务模型失败，排队内容已保留：" + err.Error()
		p.status = "failed"
		p.notifyLocked()
		p.mu.Unlock()
	}
	if _, err := p.runner.Call(ctx, "configure_model", change.Params); err != nil {
		fail(err)
		return
	}
	if _, err := s.store.UpdateConversation(id, func(row *Conversation) error { row.ModelDeferred = false; return nil }); err != nil {
		fail(err)
		return
	}
	p.mu.Lock()
	p.modelChange = nil
	p.mu.Unlock()
	if _, err := p.runner.Call(ctx, "queue", map[string]any{"action": "modelHold", "locked": false}); err != nil {
		fail(err)
		return
	}
	snapshot, err := p.runner.Call(ctx, "state", nil)
	if err == nil {
		var state struct{ Streaming bool }
		if json.Unmarshal(snapshot, &state) == nil {
			p.mu.Lock()
			p.running = state.Streaming
			p.notifyLocked()
			p.mu.Unlock()
		}
	}
	if s.options.OnActivity != nil {
		go s.options.OnActivity(id)
	}
}

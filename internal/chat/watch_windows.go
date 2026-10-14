//go:build windows

package chat

import (
	"context"
	"encoding/json"
)

// WaitEvents exposes the existing event buffer without a polling timer. Take
// the wake channel before Poll so an event arriving between reads cannot vanish.
func (s *Service) WaitEvents(ctx context.Context, id string, after uint64) (EventPage, error) {
	p, err := s.session(id)
	if err != nil {
		return EventPage{}, err
	}
	for {
		p.mu.Lock()
		if p.changed == nil {
			p.changed = make(chan struct{})
		}
		changed := p.changed
		p.mu.Unlock()
		page, err := s.Poll(id, after)
		if err != nil || page.Cursor > after || page.Reset || !page.Running {
			return page, err
		}
		select {
		case <-ctx.Done():
			return EventPage{}, ctx.Err()
		case <-changed:
		}
	}
}
func (s *Service) Cursor(id string) (uint64, error) {
	p, err := s.session(id)
	if err != nil {
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sequence, nil
}

// A late scheduler cancellation must not abort a newer user turn in a bound
// conversation. The request identity is checked under the mutation gate.
func (s *Service) StopRequest(ctx context.Context, id, requestID string) error {
	p, err := s.session(id)
	if err != nil {
		return err
	}
	if err = p.lock(ctx); err != nil {
		return err
	}
	defer p.unlock()
	p.mu.Lock()
	matches := p.running && p.activeRequest == requestID
	p.mu.Unlock()
	if !matches {
		return nil
	}
	raw, err := p.runner.Call(ctx, "abort", nil)
	if err == nil {
		err = s.remember(id, json.RawMessage(raw))
		p.mu.Lock()
		p.running = false
		p.status = "stopped"
		p.approvals = map[string]json.RawMessage{}
		p.notifyLocked()
		p.mu.Unlock()
	}
	return err
}

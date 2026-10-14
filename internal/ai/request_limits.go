package ai

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"
)

type requestLane struct {
	active  int
	sent    []time.Time
	queue   []*requestTicket
	changed chan struct{}
}
type requestTicket struct{ marker byte }
type requestScheduler struct {
	mu     sync.Mutex
	lanes  map[string]*requestLane
	window time.Duration
}

var sharedRequests requestScheduler

// A worker is short-lived, so its Magpie limiter cannot account for an earlier
// worker. Keep one host-owned lane per isolated profile and credential instead.
// Only a one-way digest of the credential is held in the in-memory lane key.
// A unit is one WinOne invocation; a Magpie-internal retry or helper request is
// part of that invocation and is still governed by upstream 429/Retry-After.
func (t Translator) acquireRequest(ctx context.Context, p Preferences, key string) (func(), error) {
	id := fmt.Sprintf("%s\x00%s\x00%x", t.DataDirectory, p.ProviderID, sha256.Sum256([]byte(key)))
	return sharedRequests.take(ctx, id, p.RequestLimits)
}

func (s *requestScheduler) take(ctx context.Context, key string, limits RequestLimits) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if limits.MaxRPM == 0 && limits.MaxConcurrency == 0 {
		return func() {}, nil
	}
	// Magpie caps a minute wait at two minutes. The caller's own cancellation
	// remains authoritative; queue wait never extends a request deadline.
	var cancel context.CancelFunc
	wait := time.Duration(limits.QueueWait) * time.Second
	if limits.MaxRPM > 0 && (wait == 0 || wait > 2*time.Minute) {
		wait = 2 * time.Minute
	}
	if wait > 0 {
		ctx, cancel = context.WithTimeout(ctx, wait)
		defer cancel()
	}
	s.mu.Lock()
	if s.lanes == nil {
		s.lanes = map[string]*requestLane{}
	}
	window := s.window
	if window == 0 {
		window = time.Minute
	}
	// Only idle expired lanes are pruned; another provider's waiting requests
	// or minute history must never be reset by a settings refresh.
	now := time.Now()
	for id, lane := range s.lanes {
		if lane.active == 0 && len(lane.queue) == 0 && (len(lane.sent) == 0 || !now.Before(lane.sent[len(lane.sent)-1].Add(window))) {
			delete(s.lanes, id)
		}
	}
	lane := s.lanes[key]
	if lane == nil {
		lane = &requestLane{changed: make(chan struct{})}
		s.lanes[key] = lane
	}
	// Nonzero-sized identity avoids Go's permitted aliasing of zero-sized pointers.
	var ticket *requestTicket
	queued := false
	wake := func() { close(lane.changed); lane.changed = make(chan struct{}) }
	remove := func() {
		if !queued {
			return
		}
		for i, item := range lane.queue {
			if item == ticket {
				lane.queue = append(lane.queue[:i], lane.queue[i+1:]...)
				break
			}
		}
		queued = false
		wake()
	}
	for {
		if err := ctx.Err(); err != nil {
			remove()
			s.mu.Unlock()
			return nil, err
		}
		now = time.Now()
		for len(lane.sent) > 0 && !now.Before(lane.sent[0].Add(window)) {
			lane.sent = lane.sent[1:]
		}
		front := len(lane.queue) == 0 || queued && lane.queue[0] == ticket
		slot := limits.MaxConcurrency == 0 || lane.active < limits.MaxConcurrency
		minute := limits.MaxRPM == 0 || len(lane.sent) < limits.MaxRPM
		if front && slot && minute {
			remove()
			lane.active++
			if limits.MaxRPM > 0 {
				lane.sent = append(lane.sent, now)
			}
			wake()
			s.mu.Unlock()
			var once sync.Once
			return func() { once.Do(func() { s.mu.Lock(); lane.active--; wake(); s.mu.Unlock() }) }, nil
		}
		if !queued {
			if limits.QueueLimit > 0 && len(lane.queue) >= limits.QueueLimit {
				s.mu.Unlock()
				return nil, errors.New("AI 请求排队已满，请稍后重试")
			}
			ticket = &requestTicket{}
			lane.queue = append(lane.queue, ticket)
			queued = true
		}
		changed := lane.changed
		var timer *time.Timer
		var ready <-chan time.Time
		if !minute {
			timer = time.NewTimer(max(time.Millisecond, time.Until(lane.sent[0].Add(window))))
			ready = timer.C
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		case <-ready:
		}
		if timer != nil {
			timer.Stop()
		}
		s.mu.Lock()
	}
}

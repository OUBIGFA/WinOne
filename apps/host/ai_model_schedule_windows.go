//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"winone/internal/ai"
)

var errMagpieAccountBusy = errors.New("account profile is in use")

// Model discovery belongs to saved providers. No schedule, worker or timer
// starts when every provider has automatic refresh disabled.
type aiModelRefresh struct {
	mu       sync.Mutex
	ctx      context.Context
	snapshot func() ([]ai.Provider, error)
	run      func(context.Context, string) error
	report   func(error)
	cancel   context.CancelFunc
	done     chan struct{}
}

func (s *aiService) startModelRefresh() {
	m := &aiModelRefresh{ctx: s.ctx, run: s.refreshProviderModels,
		snapshot: func() ([]ai.Provider, error) {
			store, err := s.magpieStore()
			if err != nil {
				return nil, err
			}
			return store.Configuration().Providers, nil
		}, report: func(err error) {
			if failure := recordHostEvent(filepath.Dir(s.storePath), "ai-model-refresh", err); failure != nil {
				fmt.Fprintln(os.Stderr, "AI model refresh diagnostics:", failure)
			}
		}}
	s.mu.Lock()
	s.modelRefresh = m
	s.mu.Unlock()
	m.Update()
}

func (s *aiService) updateModelRefresh() {
	s.mu.Lock()
	m := s.modelRefresh
	s.mu.Unlock()
	if m != nil {
		m.Update()
	}
}
func (s *aiService) stopModelRefresh() {
	s.mu.Lock()
	m := s.modelRefresh
	s.mu.Unlock()
	if m != nil {
		m.Close()
	}
}

// Join the previous pass before a saved change starts a replacement. This
// preserves the account profile's single-writer boundary across rapid saves.
func (m *aiModelRefresh) Update() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	if m.ctx.Err() != nil {
		return
	}
	providers, err := m.snapshot()
	if err != nil {
		m.log(err)
		return
	}
	var selected []ai.Provider
	for _, p := range providers {
		if !p.Off && p.ModelRefreshInterval() > 0 {
			selected = append(selected, p)
		}
	}
	if len(selected) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel, m.done = cancel, make(chan struct{})
	go m.loop(ctx, m.done, selected)
}
func (m *aiModelRefresh) Close() { m.mu.Lock(); defer m.mu.Unlock(); m.stopLocked() }
func (m *aiModelRefresh) stopLocked() {
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel, m.done = nil, nil
	}
}
func (m *aiModelRefresh) log(err error) {
	if m.report != nil {
		m.report(err)
	}
}

func (m *aiModelRefresh) loop(ctx context.Context, done chan struct{}, providers []ai.Provider) {
	defer close(done)
	next := make(map[string]time.Time, len(providers))
	for _, p := range providers {
		next[p.ID] = p.ModelRefreshAt.Add(p.ModelRefreshInterval())
		// Account discovery yields to login during startup, as it did before
		// removing the unrelated quota-maintenance service.
		if p.AuthMode == "account" && time.Now().After(next[p.ID]) {
			next[p.ID] = time.Now().Add(time.Minute)
		}
	}
	for ctx.Err() == nil {
		wake := time.Now().Add(24 * time.Hour)
		for _, p := range providers {
			if !time.Now().Before(next[p.ID]) {
				pass, cancel := context.WithTimeout(ctx, 30*time.Second)
				err := m.run(pass, p.ID)
				cancel()
				if ctx.Err() != nil {
					return
				}
				delay := p.ModelRefreshInterval()
				if err != nil {
					delay = time.Minute
					if !errors.Is(err, errMagpieAccountBusy) {
						m.log(errors.New("供应商模型自动刷新未完成"))
					}
				}
				next[p.ID] = time.Now().Add(delay)
			}
			if next[p.ID].Before(wake) {
				wake = next[p.ID]
			}
		}
		timer := time.NewTimer(max(time.Millisecond, time.Until(wake)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

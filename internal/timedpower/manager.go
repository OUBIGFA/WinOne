package timedpower

import (
	"errors"
	"sync"
	"time"
)

// Snapshot distinguishes Windows accepting a request from actually finishing
// shutdown. Cancel is only promised before the native call has been submitted.
type Snapshot struct {
	State       string       `json:"state"`
	Active      bool         `json:"active"`
	Task        *Preferences `json:"task,omitempty"`
	Deadline    time.Time    `json:"deadline"`
	Error       string       `json:"error,omitempty"`
	NoticeError string       `json:"noticeError,omitempty"`
	LastEvent   string       `json:"lastEvent,omitempty"`
}
type Manager struct {
	mu       sync.Mutex
	snapshot Snapshot
	cancel   chan struct{}
	now      func() time.Time
	execute  func(Preferences) error
	notify   func(string, Snapshot) error
	reminded bool
}

func New(execute func(Preferences) error, notify func(string, Snapshot) error) *Manager {
	return &Manager{snapshot: Snapshot{State: "idle"}, now: time.Now, execute: execute, notify: notify}
}
func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.copyLocked() }
func (m *Manager) copyLocked() Snapshot {
	s := m.snapshot
	if s.Task != nil {
		p := s.Task.Clone()
		s.Task = &p
	}
	return s
}
func (m *Manager) Start(p Preferences, confirmed bool) (Snapshot, error) {
	return m.start(p, confirmed, true)
}
func (m *Manager) start(p Preferences, confirmed, run bool) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snapshot.Active || m.snapshot.State == "executing" {
		return m.copyLocked(), errors.New("已有定时任务，请先取消")
	}
	deadline, err := p.Next(m.now())
	if err != nil {
		return m.copyLocked(), err
	}
	// Confirmation is enforced at the Host as well as the UI. Dry-run never
	// calls the executor, even if force=true or an injected executor is present.
	if !p.DryRun && !confirmed {
		return m.copyLocked(), errors.New("请确认电源操作及未保存内容丢失风险")
	}
	if !p.DryRun && m.execute == nil {
		return m.copyLocked(), errors.New("电源执行器未就绪")
	}
	p = p.Clone()
	m.snapshot = Snapshot{State: "running", Active: true, Task: &p, Deadline: deadline}
	m.reminded = false
	m.cancel = make(chan struct{})
	stop := m.cancel
	if run {
		go m.run(stop)
	}
	return m.copyLocked(), nil
}
func (m *Manager) run(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			m.tick(m.now(), stop)
		}
	}
}
func (m *Manager) Cancel() (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snapshot.State == "executing" {
		return m.copyLocked(), errors.New("操作已提交，无法再取消")
	}
	if m.snapshot.Active {
		m.finishLocked("cancelled")
	}
	return m.copyLocked(), nil
}
func (m *Manager) Close() error { _, err := m.Cancel(); return err }
func (m *Manager) finishLocked(state string) {
	m.snapshot.State = state
	m.snapshot.Active = false
	if m.cancel != nil {
		close(m.cancel)
		m.cancel = nil
	}
}
func (m *Manager) tick(now time.Time, stop <-chan struct{}) {
	m.mu.Lock()
	if m.cancel != stop || !m.snapshot.Active || m.snapshot.State == "executing" {
		m.mu.Unlock()
		return
	}
	p := m.snapshot.Task.Clone()
	remaining := m.snapshot.Deadline.Sub(now)
	if remaining > 0 {
		if !m.reminded && p.ReminderSeconds > 0 && remaining <= time.Duration(p.ReminderSeconds)*time.Second {
			m.reminded = true
			s := m.copyLocked()
			m.mu.Unlock()
			m.announce("reminder", s, stop)
			return
		}
		m.mu.Unlock()
		return
	}
	// A wake/resume or forward clock jump must not unexpectedly power off a PC
	// long after the promised time. Weekly schedules skip to the next occurrence.
	if remaining < -30*time.Second {
		m.snapshot.LastEvent = "missed"
		if p.Mode == "weekly" {
			m.snapshot.Deadline, _ = p.Next(now)
			m.reminded = false
		} else {
			m.finishLocked("missed")
		}
		s := m.copyLocked()
		m.mu.Unlock()
		m.announce("missed", s, stop)
		return
	}
	m.snapshot.State = "executing"
	s := m.copyLocked()
	m.mu.Unlock()
	var err error
	event := "submitted"
	if p.DryRun {
		event = "reminded"
	} else {
		err = m.execute(p)
	}
	m.mu.Lock()
	if err != nil {
		m.snapshot.Error = err.Error()
		event = "failed"
	}
	m.snapshot.LastEvent = event
	if p.Mode == "weekly" && err == nil {
		m.snapshot.State = "running"
		m.snapshot.Deadline, _ = p.Next(m.now())
		m.reminded = false
	} else {
		m.finishLocked(event)
	}
	s = m.copyLocked()
	m.mu.Unlock()
	m.announce(event, s, stop)
}
func (m *Manager) announce(event string, s Snapshot, stop <-chan struct{}) {
	if m.notify == nil {
		return
	}
	if err := m.notify(event, s); err != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.cancel == stop || m.cancel == nil && m.snapshot.Deadline.Equal(s.Deadline) {
			m.snapshot.NoticeError = err.Error()
		}
	}
}

package timedpower

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
	_ "time/tzdata"
)

func TestDefaultsAndValidation(t *testing.T) {
	p := Defaults()
	if !p.Force || p.Action != "shutdown" || p.DryRun {
		t.Fatal(p)
	}
	for _, change := range []func(*Preferences){func(p *Preferences) { p.Action = "cmd" }, func(p *Preferences) { p.DelaySeconds = 0 }, func(p *Preferences) { p.DelaySeconds = 604801 }, func(p *Preferences) { p.TimeOfDay = "24:00" }, func(p *Preferences) { p.ReminderSeconds = -1 }, func(p *Preferences) { p.Weekdays = []int{1, 1} }, func(p *Preferences) { p.Mode = "weekly"; p.Weekdays = nil }} {
		bad := p.Clone()
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal(bad)
		}
	}
}
func TestNextCalendarAndDST(t *testing.T) {
	now := time.Date(2026, 10, 26, 23, 30, 0, 0, time.UTC)
	p := Defaults()
	p.Mode = "time"
	got, err := p.Next(now)
	if err != nil || !got.Equal(time.Date(2026, 10, 27, 23, 0, 0, 0, time.UTC)) {
		t.Fatal(got, err)
	}
	p.Mode = "weekly"
	p.Weekdays = []int{1}
	got, err = p.Next(now)
	if err != nil || !got.Equal(time.Date(2026, 11, 2, 23, 0, 0, 0, time.UTC)) {
		t.Fatal(got, err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	p.Mode = "time"
	p.TimeOfDay = "02:30"
	got, err = p.Next(time.Date(2026, 3, 8, 0, 0, 0, 0, loc))
	if err != nil || got.Day() != 9 || got.Hour() != 2 || got.Minute() != 30 {
		t.Fatal(got, err)
	}
	p.Mode = "weekly"
	p.Weekdays = []int{0}
	got, err = p.Next(time.Date(2026, 3, 1, 3, 0, 0, 0, loc))
	if err != nil || got.Day() != 15 || got.Hour() != 2 || got.Minute() != 30 {
		t.Fatal("weekly DST gap must advance two weeks", got, err)
	}
}
func TestArmingConfirmationAndCancel(t *testing.T) {
	calls := 0
	m := New(func(Preferences) error { calls++; return nil }, nil)
	p := Defaults()
	if _, err := m.start(p, false, false); err == nil {
		t.Fatal("unconfirmed real action accepted")
	}
	if _, err := m.start(p, true, false); err != nil {
		t.Fatal(err)
	}
	stop := m.cancel
	snapshot := m.Snapshot()
	snapshot.Task.Weekdays[0] = 6
	if m.Snapshot().Task.Weekdays[0] != 1 {
		t.Fatal("mutable snapshot")
	}
	if _, err := m.start(p, true, false); err == nil {
		t.Fatal("duplicate start")
	}
	if _, err := m.Cancel(); err != nil {
		t.Fatal(err)
	}
	m.tick(snapshot.Deadline, stop)
	if calls != 0 || m.Snapshot().Active || m.Snapshot().State != "cancelled" {
		t.Fatal(m.Snapshot(), calls)
	}
}
func TestDryRunAndReminderNeverExecute(t *testing.T) {
	calls := 0
	events := []string{}
	m := New(func(Preferences) error { calls++; return nil }, func(event string, _ Snapshot) error { events = append(events, event); return nil })
	p := Defaults()
	p.DryRun = true
	p.DelaySeconds = 100
	if _, err := m.start(p, false, false); err != nil {
		t.Fatal(err)
	}
	deadline := m.Snapshot().Deadline
	stop := m.cancel
	m.tick(deadline.Add(-40*time.Second), stop)
	m.tick(deadline.Add(-20*time.Second), stop)
	m.tick(deadline, stop)
	m.tick(deadline.Add(time.Second), stop)
	if calls != 0 || len(events) != 2 || events[0] != "reminder" || events[1] != "reminded" || m.Snapshot().Active {
		t.Fatal(calls, events, m.Snapshot())
	}
}
func TestExecuteOnceFailuresAndMissedSchedule(t *testing.T) {
	for _, fail := range []bool{false, true} {
		calls := 0
		m := New(func(p Preferences) error {
			calls++
			if !p.Force {
				t.Fatal("lost force")
			}
			if fail {
				return errors.New("denied")
			}
			return nil
		}, nil)
		if _, err := m.start(Defaults(), true, false); err != nil {
			t.Fatal(err)
		}
		stop := m.cancel
		deadline := m.Snapshot().Deadline
		m.tick(deadline, stop)
		m.tick(deadline, stop)
		expected := "submitted"
		if fail {
			expected = "failed"
		}
		if calls != 1 || m.Snapshot().State != expected {
			t.Fatal(calls, m.Snapshot())
		}
	}
	calls := 0
	m := New(func(Preferences) error { calls++; return nil }, nil)
	if _, err := m.start(Defaults(), true, false); err != nil {
		t.Fatal(err)
	}
	m.tick(m.Snapshot().Deadline.Add(31*time.Second), m.cancel)
	if calls != 0 || m.Snapshot().State != "missed" {
		t.Fatal(m.Snapshot())
	}
}
func TestWeeklyRepeatsAndSkipsMissed(t *testing.T) {
	now := time.Date(2026, 10, 26, 22, 0, 0, 0, time.UTC)
	m := New(nil, nil)
	m.now = func() time.Time { return now }
	p := Defaults()
	p.DryRun = true
	p.Mode = "weekly"
	p.Weekdays = []int{1}
	if _, err := m.start(p, false, false); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	stop := m.cancel
	now = m.Snapshot().Deadline
	m.tick(now, stop)
	if !m.Snapshot().Active || m.Snapshot().Deadline.Sub(now) != 7*24*time.Hour {
		t.Fatal(m.Snapshot())
	}
	now = m.Snapshot().Deadline.Add(time.Hour)
	m.tick(now, stop)
	if !m.Snapshot().Active || m.Snapshot().LastEvent != "missed" || !m.Snapshot().Deadline.After(now) {
		t.Fatal(m.Snapshot())
	}
}
func TestCancelDuringSubmissionReportsTooLate(t *testing.T) {
	began, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := New(func(Preferences) error { close(began); <-finish; return nil }, nil)
	if _, err := m.start(Defaults(), true, false); err != nil {
		t.Fatal(err)
	}
	go func() { m.tick(m.Snapshot().Deadline, m.cancel); close(done) }()
	<-began
	if _, err := m.Cancel(); err == nil {
		t.Fatal("claimed to cancel submitted call")
	}
	close(finish)
	<-done
	if m.Snapshot().State != "submitted" {
		t.Fatal(m.Snapshot())
	}
}
func TestRealTimerLoopDryRunAndNotificationFailure(t *testing.T) {
	var calls atomic.Int32
	m := New(func(Preferences) error { calls.Add(1); return nil }, func(string, Snapshot) error { return errors.New("notifications disabled") })
	p := Defaults()
	p.DryRun = true
	p.DelaySeconds = 1
	if _, err := m.Start(p, false); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Snapshot()
		if !s.Active && s.NoticeError != "" {
			if calls.Load() != 0 || s.State != "reminded" {
				t.Fatal(s)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(m.Snapshot())
}

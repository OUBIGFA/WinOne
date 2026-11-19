package core

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/internal/timedpower"
)

// TimedPower shares one live task between the page and the global sidebar.
// Reads run off the UI thread. An idle task never keeps a polling timer alive.
type TimedPower struct {
	env        *Env
	Status     *timedpower.Snapshot
	Err        string
	updated    time.Time
	reading    bool
	generation uint64
	closed     bool
}

func NewTimedPower(env *Env) *TimedPower { return &TimedPower{env: env} }

func (p *TimedPower) Read() {
	if p.reading || p.closed {
		return
	}
	p.reading = true
	version := p.generation
	Go(p.env, func() (timedpower.Snapshot, error) {
		var s timedpower.Snapshot
		err := p.env.Host.CallInto("timed-power.snapshot", nil, &s)
		return s, err
	}, func(s timedpower.Snapshot, err error) {
		p.reading = false
		if p.closed || version != p.generation {
			return
		}
		p.updated = time.Now()
		if err != nil {
			p.Err = err.Error()
			return
		}
		p.Update(s)
	})
}

// Invalidate prevents an older read from undoing a start/cancel operation.
func (p *TimedPower) Invalidate() { p.generation++ }

func (p *TimedPower) Update(s timedpower.Snapshot) {
	p.Invalidate()
	p.Status = &s
	p.Err = ""
	p.updated = time.Now()
}

func (p *TimedPower) Clear() { p.Invalidate(); p.Status = nil; p.Err = "" }
func (p *TimedPower) Close() { p.closed = true; p.Clear() }

// Frame is safe to call from both consumers; reading prevents duplicate work.
func (p *TimedPower) Frame(c *ui.Context) {
	if p.closed || p.Status == nil || !p.Status.Active {
		return
	}
	c.After(time.Second)
	if time.Since(p.updated) >= time.Second {
		p.Read()
	}
}

func PowerRemaining(deadline, now time.Time) string {
	seconds := int(math.Ceil(max(0, deadline.Sub(now).Seconds())))
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

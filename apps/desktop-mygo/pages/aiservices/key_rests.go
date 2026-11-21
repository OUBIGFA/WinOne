package aiservices

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type keyRest struct {
	Why, Error, By string
	Status         int
	Until          time.Time
	Probed, Rate   bool
}

// Live cooldown state stays outside the configuration draft. Only an explicit
// read opens the private runtime; switching providers cancels its pending read.
type keyRestPanel struct {
	api               nativeAPI
	requested, loaded bool
	rests             map[string]*keyRest
	groupRates        map[string]*float64
}

func (s *keyRestPanel) accept(raw json.RawMessage) {
	var state struct {
		Keys []struct {
			ID        string
			Rest      *keyRest
			GroupRate *float64
		}
	}
	if nativeDecode(&s.api, raw, &state) {
		s.rests = map[string]*keyRest{}
		s.groupRates = map[string]*float64{}
		for _, key := range state.Keys {
			s.rests[key.ID] = key.Rest
			s.groupRates[key.ID] = key.GroupRate
		}
		s.loaded = true
	}
}

func (e *editor) keyRestControls(c *ui.Context, pr *provider, locked bool) {
	s := &e.keyRests
	if kit.GhostButton(c, i18n.T("aiSync.readKeyState"), icons.RefreshCw, s.api.busy).
		Disabled(locked || s.api.busy || !e.p.connectionSaved(pr) || len(pr.Models) == 0).Clicked() {
		s.requested, s.loaded = true, false
		s.api.err = ""
		e.p.gateway.ensureRuntime()
	}
	if s.requested && !s.loaded && !s.api.busy && s.api.err == "" {
		if e.p.gateway.runtimeURL != "" {
			s.api.request(e.p, "GET", "/api/winone/providers/"+pr.ID+"/key-rests", nil, s.accept)
		} else if e.p.gateway.runtimeError != "" {
			s.api.err = e.p.gateway.runtimeError
		} else {
			c.After(100 * time.Millisecond)
		}
	}
	s.api.notice(c, nil)
}

func (e *editor) keyRestRow(c *ui.Context, pr *provider, slot string, locked bool) {
	s := &e.keyRests
	if rate := s.groupRates[slot]; rate != nil {
		kit.Help(c, upstreamT("Read from the relay: {rates}", "rates", fmt.Sprintf("×%g", *rate)))
	}
	r := s.rests[slot]
	if r == nil {
		return
	}
	key := "aiSync.keyResting"
	if r.Rate || r.Why == "rate" {
		key = "aiSync.keyRateLimited"
	}
	if r.Why == "suspended" {
		key = "aiSync.keySuspended"
	}
	kit.Help(c, fmt.Sprintf("%s · HTTP %d · %s %s", i18n.T(key), r.Status, i18n.T("aiSync.keyResumes"), r.Until.Local().Format("01-02 15:04")))
	if r.Error != "" {
		kit.Help(c, r.Error).Selectable()
	}
	if r.Probed {
		kit.Help(c, i18n.T("aiSync.keyAutoProbe"))
	}
	if kit.GhostButton(c, i18n.T("aiSync.retryKeyNow"), icons.RotateCcw, s.api.busy).Disabled(locked || s.api.busy).Clicked() {
		s.api.request(e.p, "POST", "/api/winone/providers/"+pr.ID+"/key-rests/retry", map[string]string{"id": slot}, s.accept)
	}
}

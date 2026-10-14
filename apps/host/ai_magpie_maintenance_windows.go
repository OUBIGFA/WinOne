//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"fyne.io/systray"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"winone/internal/ai"
)

var errMagpieAccountBusy = errors.New("account profile is in use")

// One Host owns the schedule; each vendor operation is a bounded worker. The
// callbacks keep native presentation out of ordinary tests and quota parsing.
type aiMagpieMaintenance struct {
	mu         sync.Mutex
	ctx        context.Context
	snapshot   func() ([]ai.Provider, *ai.MagpieSettings, error)
	runAccount func(context.Context, ai.Provider, *ai.MagpieSettings) (json.RawMessage, error)
	locale     func() string
	tooltip    func(string)
	notify     func(string, string) error
	report     func(error)
	wait       func(context.Context, time.Duration) bool
	cancel     context.CancelFunc
	done       chan struct{}
}

func (s *aiService) startMagpieMaintenance(locale func() string) {
	m := &aiMagpieMaintenance{ctx: s.ctx, locale: locale, tooltip: systray.SetTooltip, notify: systray.ShowWarning,
		runAccount: s.maintainMagpieAccount, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
			store, err := s.magpieStore()
			if err != nil {
				return nil, nil, err
			}
			prefs, err := store.MagpieRuntime()
			return store.Configuration().Providers, prefs, err
		}, report: func(err error) {
			if logErr := recordHostEvent(filepath.Dir(s.storePath), "ai-quota-maintenance", err); logErr != nil {
				fmt.Fprintln(os.Stderr, "AI quota diagnostics:", logErr)
			}
		}}
	s.mu.Lock()
	s.maintenance = m
	s.mu.Unlock()
	m.Update()
}

func (s *aiService) refreshMagpieMaintenance() {
	s.mu.Lock()
	m := s.maintenance
	s.mu.Unlock()
	if m != nil {
		m.Update()
	}
}

func (s *aiService) stopMagpieMaintenance() {
	s.mu.Lock()
	m := s.maintenance
	s.mu.Unlock()
	if m != nil {
		m.Close()
	}
}

func (s *aiService) maintainMagpieAccount(ctx context.Context, p ai.Provider, prefs *ai.MagpieSettings) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gate := s.accountGateFor(p.ID)
	// Background upkeep yields to a live login/model request. It can try again
	// next tick without claiming that contention means a broken account.
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	default:
		return nil, errMagpieAccountBusy
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.translator.Maintain(ctx, p, prefs)
}

// Update joins the cancelled pass before admitting a replacement. A save can
// never leave two workers writing the same private account profile.
func (m *aiMagpieMaintenance) Update() {
	m.mu.Lock()
	defer m.mu.Unlock()
	wasRunning := m.done != nil
	m.stopLocked()
	if m.ctx.Err() != nil {
		return
	}
	providers, prefs, err := m.snapshot()
	if err != nil {
		m.log(err)
		return
	}
	accounts := []ai.Provider{}
	for _, p := range providers {
		if p.AuthMode == "account" {
			accounts = append(accounts, p)
		}
	}
	if !prefs.MaintenanceNeeded() || len(accounts) == 0 {
		if wasRunning && m.tooltip != nil {
			m.tooltip("WinOne")
		}
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.loop(ctx, m.done, accounts, prefs)
}

func (m *aiMagpieMaintenance) Close() { m.mu.Lock(); defer m.mu.Unlock(); m.stopLocked() }
func (m *aiMagpieMaintenance) stopLocked() {
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel = nil
		m.done = nil
	}
}
func (m *aiMagpieMaintenance) log(err error) {
	if err != nil && m.report != nil {
		m.report(err)
	}
}
func (m *aiMagpieMaintenance) language() string {
	if m.locale != nil {
		return m.locale()
	}
	return "zh-CN"
}

func maintenanceWait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func magpieTrayInterval(p *ai.MagpieSettings) time.Duration {
	if p.TrayUsageEvery <= 0 {
		return 3 * time.Minute
	}
	return time.Duration(p.TrayUsageEvery) * time.Minute
}
func magpieMaintenanceInterval(p *ai.MagpieSettings) time.Duration {
	withoutTray := *p
	withoutTray.TrayUsages = nil
	if withoutTray.MaintenanceNeeded() {
		return time.Minute
	}
	return magpieTrayInterval(p)
}

func (m *aiMagpieMaintenance) loop(ctx context.Context, done chan struct{}, accounts []ai.Provider, prefs *ai.MagpieSettings) {
	defer close(done)
	wait := m.wait
	if wait == nil {
		wait = maintenanceWait
	}
	cache := map[string][]magpieQuota{}
	readAt := map[string]time.Time{}
	failures := map[string]string{}
	var lastTip time.Time
	for {
		staleChanged := false
		for _, p := range accounts {
			if ctx.Err() != nil {
				return
			}
			raw, err := m.runAccount(ctx, p, prefs)
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, errMagpieAccountBusy) {
				continue
			}
			var result magpieMaintenanceResult
			if err == nil {
				err = json.Unmarshal(raw, &result)
			}
			if err != nil {
				// Do not put response bodies, private credentials or raw upstream
				// transport failures in desktop notifications or diagnostics.
				issue := p.Name + ": 额度维护未完成"
				if failures[p.ID] != issue {
					m.log(errors.New(issue))
					failures[p.ID] = issue
				}
				// Preserve the last useful reading, but never continue presenting
				// it as fresh after a failed worker or vendor refresh.
				for i := range cache[p.ID] {
					if cache[p.ID][i].AsOf == nil {
						at := readAt[p.ID]
						cache[p.ID][i].AsOf = &at
						staleChanged = true
					}
				}
				continue
			}
			cache[p.ID] = result.TrayCards
			readAt[p.ID] = time.Now()
			if len(result.Errors) > 0 {
				issue := fmt.Sprintf("%s: %d 项额度维护操作未完成", p.Name, len(result.Errors))
				if failures[p.ID] != issue {
					m.log(errors.New(issue))
					failures[p.ID] = issue
				}
			} else {
				delete(failures, p.ID)
			}
			for _, alert := range result.Alerts {
				if ctx.Err() != nil {
					return
				}
				title, body := magpieAlertText(alert, prefs, m.language())
				if title != "" && body != "" && m.notify != nil {
					m.log(m.notify(title, body))
				}
			}
		}
		if len(prefs.TrayUsages) > 0 && m.tooltip != nil && (staleChanged || lastTip.IsZero() || time.Since(lastTip) >= magpieTrayInterval(prefs)) {
			cards := []magpieQuota{}
			for _, p := range accounts {
				cards = append(cards, cache[p.ID]...)
			}
			m.tooltip(magpieTooltip(cards, prefs, m.language()))
			lastTip = time.Now()
		}
		if !wait(ctx, magpieMaintenanceInterval(prefs)) {
			return
		}
	}
}

// The worker uses upstream's typed quota/card selection before serializing:
// JSON omits internal Model/Aside/Span fields needed to filter tray windows.
type magpieMaintenanceResult struct {
	TrayCards []magpieQuota `json:"trayCards"`
	Alerts    []magpieAlert `json:"alerts"`
	Errors    []string      `json:"errors"`
}
type magpieQuota struct {
	Provider string              `json:"provider"`
	Name     string              `json:"name"`
	User     string              `json:"user"`
	Windows  []magpieQuotaWindow `json:"windows"`
	Balance  string              `json:"balance"`
	Error    string              `json:"error"`
	AsOf     *time.Time          `json:"asOf"`
}
type magpieQuotaWindow struct {
	Name      string     `json:"name"`
	Used      float64    `json:"used"`
	Amount    float64    `json:"amount"`
	Limit     float64    `json:"limit"`
	Unit      string     `json:"unit"`
	ResetsAt  *time.Time `json:"resetsAt"`
	ResetSecs int64      `json:"resetSecs"`
}
type magpieAlert struct {
	Provider, Name, User, Window string
	Used                         float64
	ResetsAt                     *time.Time
	Balance, Kind                string
	Credits                      int
}

// Win11 has no menu-bar text cells. Preserve Magpie's Windows tooltip behavior
// using the existing native tray, including usage direction and stale readings.
func magpieTooltip(cards []magpieQuota, prefs *ai.MagpieSettings, locale string) string {
	lines := []string{"WinOne"}
	english := locale == "en-US"
	for _, q := range cards {
		name := q.Name
		if name == "" {
			name = q.Provider
		}
		if prefs.HideAccounts && q.User != "" {
			name = strings.ReplaceAll(name, q.User, "•••")
		}
		if q.Error != "" {
			if english {
				lines = append(lines, name+": unavailable")
			} else {
				lines = append(lines, name+"：暂不可用")
			}
			continue
		}
		parts := []string{}
		for _, w := range q.Windows {
			used := math.Max(0, math.Min(100, w.Used))
			word := "已用"
			if english {
				word = "used"
			}
			if prefs.QuotaLeft {
				used = 100 - used
				word = "剩余"
				if english {
					word = "left"
				}
			}
			amount := ""
			if w.Limit > 0 {
				n := w.Amount
				if prefs.QuotaLeft {
					n = math.Max(0, w.Limit-n)
				}
				amount = fmt.Sprintf("%g/%g %s · ", n, w.Limit, w.Unit)
			}
			line := fmt.Sprintf("%s %s%s %s", w.Name, amount, magpiePercent(used), word)
			if w.ResetsAt != nil {
				line += " · " + w.ResetsAt.Local().Format("01-02 15:04")
			} else if w.ResetSecs > 0 {
				line += fmt.Sprintf(" · %dm", max(1, w.ResetSecs/60))
			}
			parts = append(parts, line)
		}
		if len(parts) == 0 && q.Balance != "" {
			parts = append(parts, q.Balance)
		}
		if len(parts) == 0 {
			continue
		}
		line := name + ": " + strings.Join(parts, "; ")
		if q.AsOf != nil {
			if english {
				line += " (cached "
			} else {
				line += "（缓存 "
			}
			line += q.AsOf.Local().Format("15:04") + ")"
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		if english {
			lines = append(lines, "Allowance unavailable")
		} else {
			lines = append(lines, "额度暂不可用")
		}
	}
	return boundedTrayText(strings.Join(lines, "\n"), 127)
}

func boundedTrayText(text string, limit int) string {
	text = strings.ReplaceAll(text, "\x00", "")
	units := utf16.Encode([]rune(text))
	if len(units) <= limit {
		return text
	}
	units = units[:limit-1]
	if n := len(units); n > 0 && units[n-1] >= 0xd800 && units[n-1] <= 0xdbff {
		units = units[:n-1]
	}
	return string(utf16.Decode(units)) + "…"
}
func magpiePercent(n float64) string { return fmt.Sprintf("%g%%", math.Round(n*10)/10) }

// Message semantics follow Magpie gui/quota_alert.go; WinOne supplies its two
// supported locales and the already-selected account masking preference.
func magpieAlertText(a magpieAlert, prefs *ai.MagpieSettings, locale string) (string, string) {
	title := a.Name
	if title == "" {
		title = a.Provider
	}
	if title == "" {
		return "", ""
	}
	if a.User != "" && !prefs.HideAccounts {
		title += " · " + a.User
	}
	english := locale == "en-US"
	at := ""
	if a.ResetsAt != nil {
		at = a.ResetsAt.Local().Format("01-02 15:04")
	}
	if a.Kind == "expires" {
		if at == "" {
			return "", ""
		}
		if english {
			return title, fmt.Sprintf("%d unused resets expire %s", a.Credits, at)
		}
		return title, fmt.Sprintf("%d 张重置卡尚未使用，将于 %s 过期", a.Credits, at)
	}
	if a.Kind == "renews" {
		if at == "" {
			return "", ""
		}
		pct := magpiePercent(math.Max(0, 100-a.Used))
		if english {
			return title, fmt.Sprintf("%s: %s left, renews %s", a.Window, pct, at)
		}
		return title, fmt.Sprintf("%s 窗口还剩 %s，%s 重置", a.Window, pct, at)
	}
	if a.Kind != "" {
		return "", ""
	}
	if a.Window == "" {
		if a.Balance == "" {
			return "", ""
		}
		if english {
			return title, fmt.Sprintf("Balance down to %s (alert at %g)", a.Balance, prefs.BalanceAlert)
		}
		return title, fmt.Sprintf("余额已降至 %s（提醒线 %g）", a.Balance, prefs.BalanceAlert)
	}
	n := a.Used
	word := "已用"
	if english {
		word = "used"
	}
	if prefs.QuotaLeft {
		n = math.Max(0, 100-n)
		word = "剩余"
		if english {
			word = "left"
		}
	}
	body := fmt.Sprintf("%s: %s %s", a.Window, magpiePercent(n), word)
	if at != "" {
		if english {
			body += ", resets " + at
		} else {
			body += "，" + at + " 重置"
		}
	}
	return title, body
}

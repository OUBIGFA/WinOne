//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestMagpieMaintenanceDefaultStartsNoWorkOrTimer(t *testing.T) {
	m := &aiMagpieMaintenance{ctx: context.Background(), snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		return []ai.Provider{{ID: "one", AuthMode: "account"}}, &ai.MagpieSettings{}, nil
	}, runAccount: func(context.Context, ai.Provider, *ai.MagpieSettings) (json.RawMessage, error) {
		t.Error("default settings started a worker")
		return nil, nil
	}, wait: func(context.Context, time.Duration) bool { t.Error("default settings started a timer"); return false }}
	m.Update()
	defer m.Close()
	if m.done != nil {
		t.Fatal("default settings retained a maintenance goroutine")
	}
}

func TestMagpieMaintenanceFollowsSuccessfulConfigurationSave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := queuedAccountService(t, ctx)
	started, stopped := make(chan struct{}), make(chan struct{})
	m := &aiMagpieMaintenance{ctx: ctx, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		p, err := s.store.MagpieRuntime()
		return s.store.Configuration().Providers, p, err
	}, runAccount: func(ctx context.Context, _ ai.Provider, _ *ai.MagpieSettings) (json.RawMessage, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}}
	s.maintenance = m
	defer m.Close()
	save := func(enabled bool) {
		config := s.store.Configuration()
		config.MagpieSettings = &ai.MagpieSettings{}
		if enabled {
			config.MagpieSettings.UsageAlert = 80
		}
		raw, _ := json.Marshal(map[string]any{"configuration": config})
		r := s.handle(host.Request{ID: "save", Method: "configuration.save", Params: raw})
		if r.Error != nil {
			t.Fatal(r.Error)
		}
	}
	save(true)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("saving enabled settings did not start maintenance")
	}
	save(false)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("saving disabled settings kept the worker")
	}
}

func TestMagpieMaintenanceSerializesAccountsAndCancelsOldSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, ended := make(chan string, 8), make(chan string, 8)
	var active, maximum atomic.Int32
	prefs := &ai.MagpieSettings{UsageAlert: 80}
	m := &aiMagpieMaintenance{ctx: ctx, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		return []ai.Provider{{ID: "api", AuthMode: "api-key"}, {ID: "first", AuthMode: "account"}, {ID: "second", AuthMode: "account"}}, prefs, nil
	}, runAccount: func(ctx context.Context, p ai.Provider, _ *ai.MagpieSettings) (json.RawMessage, error) {
		n := active.Add(1)
		maximum.Store(max(maximum.Load(), n))
		defer active.Add(-1)
		started <- p.ID
		if p.ID == "first" {
			<-ctx.Done()
			ended <- p.ID
			return nil, ctx.Err()
		}
		return json.RawMessage(`{"trayCards":[],"alerts":[],"errors":[]}`), nil
	}}
	m.Update()
	if got := <-started; got != "first" {
		t.Fatal(got)
	}
	prefs = &ai.MagpieSettings{}
	m.Update()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("disabled settings did not cancel the active pass")
	}
	m.Close()
	if active.Load() != 0 || maximum.Load() != 1 || len(started) != 0 {
		t.Fatal("workers overlapped or a disabled pass kept starting accounts")
	}
}

func TestMagpieMaintenancePreservesIntervalAndPresentsRealResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tips, notices, intervals := make(chan string, 4), make(chan string, 4), make(chan time.Duration, 4)
	m := &aiMagpieMaintenance{ctx: ctx, locale: func() string { return "zh-CN" }, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		return []ai.Provider{{ID: "one", Name: "Account", AuthMode: "account"}}, &ai.MagpieSettings{TrayUsages: []string{"codex|a"}, TrayUsageEvery: 5, QuotaLeft: true}, nil
	}, runAccount: func(context.Context, ai.Provider, *ai.MagpieSettings) (json.RawMessage, error) {
		return json.RawMessage(`{"trayCards":[{"provider":"codex","name":"Codex","user":"a","windows":[{"name":"5 hours","used":82.3}]}],"alerts":[{"Name":"Codex","Window":"5 hours","Used":82.3}],"checkins":[],"errors":[]}`), nil
	}, tooltip: func(s string) { tips <- s }, notify: func(title, body string) error { notices <- title + " " + body; return nil }, wait: func(ctx context.Context, interval time.Duration) bool {
		intervals <- interval
		<-ctx.Done()
		return false
	}}
	m.Update()
	defer m.Close()
	select {
	case tip := <-tips:
		if !strings.Contains(tip, "17.7%") || !strings.Contains(tip, "剩余") {
			t.Fatal(tip)
		}
	case <-time.After(time.Second):
		t.Fatal("quota result never reached tray")
	}
	select {
	case message := <-notices:
		if !strings.Contains(message, "17.7%") {
			t.Fatal(message)
		}
	case <-time.After(time.Second):
		t.Fatal("alert never reached notifier")
	}
	if interval := <-intervals; interval != 5*time.Minute {
		t.Fatal(interval)
	}
}

func TestMagpieMaintenanceDoesNotNotifyOnErrorsOrCancelledOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var notified atomic.Int32
	errorsSeen := make(chan error, 2)
	m := &aiMagpieMaintenance{ctx: ctx, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		return []ai.Provider{{ID: "one", AuthMode: "account"}}, &ai.MagpieSettings{UsageAlert: 80}, nil
	},
		runAccount: func(context.Context, ai.Provider, *ai.MagpieSettings) (json.RawMessage, error) {
			return nil, errors.New("private worker error")
		},
		notify: func(string, string) error { notified.Add(1); return nil }, report: func(err error) { errorsSeen <- err }, wait: func(context.Context, time.Duration) bool { return false }}
	m.Update()
	<-errorsSeen
	m.Close()
	if notified.Load() != 0 {
		t.Fatal("failed maintenance displayed a successful alert")
	}
	started := make(chan struct{})
	m.runAccount = func(ctx context.Context, _ ai.Provider, _ *ai.MagpieSettings) (json.RawMessage, error) {
		close(started)
		<-ctx.Done()
		return json.RawMessage(`{"alerts":[{"Name":"stale","Balance":"0"}]}`), nil
	}
	m.Update()
	<-started
	m.Close()
	if notified.Load() != 0 {
		t.Fatal("cancelled maintenance displayed a stale alert")
	}
}

func TestMagpieMaintenanceUsesAccountGateAndReleasesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := queuedAccountService(t, ctx)
	gate := s.accountGateFor("account")
	gate <- struct{}{}
	if _, err := s.maintainMagpieAccount(ctx, ai.Provider{ID: "account", AuthMode: "account"}, &ai.MagpieSettings{UsageAlert: 80}); !errors.Is(err, errMagpieAccountBusy) {
		t.Fatal(err)
	}
	if len(gate) != 1 {
		t.Fatal("maintenance displaced another account owner")
	}
	<-gate
	cancel()
	if _, err := s.maintainMagpieAccount(ctx, ai.Provider{ID: "account", AuthMode: "account"}, &ai.MagpieSettings{UsageAlert: 80}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(gate) != 0 {
		t.Fatal("cancelled maintenance retained the gate")
	}
}

func TestMagpieMaintenanceMarksLastQuotaStaleWhenRefreshFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tips := make(chan string, 4)
	calls := 0
	waits := 0
	m := &aiMagpieMaintenance{ctx: ctx, locale: func() string { return "en-US" }, snapshot: func() ([]ai.Provider, *ai.MagpieSettings, error) {
		return []ai.Provider{{ID: "one", AuthMode: "account"}}, &ai.MagpieSettings{UsageAlert: 80, TrayUsages: []string{"codex|a"}}, nil
	},
		runAccount: func(context.Context, ai.Provider, *ai.MagpieSettings) (json.RawMessage, error) {
			calls++
			if calls == 1 {
				return json.RawMessage(`{"trayCards":[{"name":"Codex","windows":[{"name":"week","used":25}]}],"alerts":[],"errors":[]}`), nil
			}
			return nil, errors.New("worker unavailable")
		},
		tooltip: func(s string) { tips <- s }, wait: func(context.Context, time.Duration) bool { waits++; return waits == 1 }}
	m.Update()
	select {
	case <-tips:
	case <-time.After(time.Second):
		t.Fatal("first quota missing")
	}
	select {
	case text := <-tips:
		if !strings.Contains(text, "cached") || !strings.Contains(text, "25%") {
			t.Fatal(text)
		}
	case <-time.After(time.Second):
		t.Fatal("failed refresh left the tray looking fresh")
	}
	m.Close()
}

func TestMagpieTrayTextAndAlertsKeepWindowsSemantics(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
	prefs := &ai.MagpieSettings{HideAccounts: true, QuotaLeft: true, BalanceAlert: 5}
	for _, locale := range []string{"zh-CN", "en-US"} {
		for _, a := range []magpieAlert{{Name: "Codex", User: "private@example.test", Window: "week", Used: 80, Kind: "renews", ResetsAt: &at}, {Name: "Codex", Kind: "expires", Credits: 2, ResetsAt: &at}, {Name: "API", Balance: "$4.20"}} {
			title, body := magpieAlertText(a, prefs, locale)
			if title == "" || body == "" || strings.Contains(title, "private@example.test") {
				t.Fatal(title, body)
			}
		}
	}
	text := magpieTooltip([]magpieQuota{{Name: strings.Repeat("🪶", 100), Windows: []magpieQuotaWindow{{Name: "week", Used: 12}}}}, prefs, "en-US")
	if len(utf16.Encode([]rune(text))) > 127 || strings.ContainsRune(text, '\x00') {
		t.Fatal("Windows tooltip is not bounded safely")
	}
}

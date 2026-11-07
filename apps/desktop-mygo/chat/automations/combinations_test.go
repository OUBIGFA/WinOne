package automations

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"

	"strings"
	"testing"
	"time"
)

func TestClearingTheSelectionRejectsAnOutstandingRunHistory(t *testing.T) {
	f, h, p, tt := setup(t, 900)
	entered, release := make(chan struct{}), make(chan struct{})
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.automations.runs" {
			close(entered)
			<-release
			return []Run{{ID: "stale", SessionID: "wrong-owner"}}, nil
		}
		return f.handle(method, raw)
	})
	p.readRuns()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("history read never started")
	}
	p.selected = ""
	p.readRuns()
	close(release)
	h.Settle(tt)
	if len(p.runs) != 0 || p.runsID != "" || p.runsLoading {
		t.Fatalf("empty selection accepted stale history: %s %+v loading=%v", p.runsID, p.runs, p.runsLoading)
	}
}

func TestSwitchingSchedulesNeverShowsThePreviousOwnersRunActions(t *testing.T) {
	_, h, p, tt := setup(t, 900)
	p.selected = "second"
	p.items = append(p.items, Schedule{ID: "second", Definition: Definition{Name: "Second schedule"}})
	p.runsLoading = true
	// The existing history belongs to s1; it must not offer its delete/jump
	// actions under the newly selected schedule, even if the new read fails.
	if p.runsID != "s1" {
		t.Fatal("fixture did not load the original owner")
	}
	// historyView uses the exact same run ownership gate as the full page.
	p.runsGeneration++
	p.readRuns()
	if len(p.runs) != 0 {
		t.Fatal("switch kept the previous schedule's run controls")
	}
	h.Settle(tt)
}

func TestPersistentHistoryStartsAtCurrentPreviewAndKeepsOlderExpansion(t *testing.T) {
	_, h, p, _ := setup(t, 900)
	p.runs = []Run{{ID:"current-one",SessionID:"current-owner",ResultText:"Current reply",Status:"success"},{ID:"previous-one",SessionID:"previous-owner",ResultText:"Older reply",Status:"success"}}
	schedule := &Schedule{Definition:Definition{PersistentSession:true}}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(),false)); p.historyGroupsView(c,schedule) },900,650)
	h.Settle(tt)
	if !tt.HasText("Current reply") || tt.HasText("Older reply") { t.Fatal("older persistent history was not initially collapsed") }
	label := tr("conversationRuns","count",1)+" · previous"
	if err := tt.Click(label); err != nil { t.Fatal(err) }
	h.Settle(tt)
	if !tt.HasText("Older reply") { t.Fatal("persistent group header did not expand its history") }
	p.runs = append([]Run{{ID:"current-two",SessionID:"current-owner",ResultText:"A new run",Status:"success"}},p.runs...)
	h.Settle(tt)
	if !tt.HasText("Older reply") { t.Fatal("a new run reset another conversation's expanded history") }
}

func TestOpeningRunMarksTheExactReceiptReadAndRejectsMissingSessions(t *testing.T) {
	f, h, p, tt := setup(t, 900)
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) { if method == "chat.automations.markRead" { return map[string]bool{"ok":true}, nil }; return f.handle(method,raw) })
	opened := ""
	p.options.OpenSession = func(id string) { opened = id }
	run := Run{ID:"r1",SessionID:"owner",Status:"success"}
	tt = ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(),false)); p.runView(c,run) },900,400)
	h.Settle(tt)
	if err := tt.Click(tr("openSession")); err != nil { t.Fatal(err) }
	for range 3 { h.Settle(tt) }
	if opened != "owner" || len(h.Called("chat.automations.markRead"))!=1 || !strings.Contains(string(h.Called("chat.automations.markRead")[0].Params),`"runId":"r1"`) { t.Fatal("opening a run did not mark its exact receipt read") }
	run = Run{ID:"missing-run",SessionID:"missing",Status:"failed"}
	h.Settle(tt)
	if err := tt.Click(tr("openSession")); err != nil { t.Fatal(err) }
	for range 3 { h.Settle(tt) }
	if opened != "owner" || len(h.Called("chat.automations.markRead"))!=1 || p.err=="" { t.Fatal("missing session was opened or marked read") }
}

func TestTemplateHydratesItsTimingAndTimeoutBuffersWithoutLosingDefaults(t *testing.T) {
	for _, sample := range []struct{ name, raw, timing, interval, scriptTimeout, hookTimeout string }{
		{"manual", `{"name":"Manual template","prompt":"Inspect","manual":true}`, "manual", "", "", ""},
		{"interval", `{"name":"Interval template","prompt":"Inspect","intervalMs":7200000}`, "interval", "120", "", ""},
		{"date", `{"name":"Date template","prompt":"Inspect","onceAt":1900000000000}`, "date", "", "", ""},
		{"timeouts", `{"name":"Script template","executionMode":"script","scriptConfig":{"command":"fixture command","timeoutMs":5000,"capabilities":[]},"preRunHook":{"command":"fixture check","timeoutMs":2000}}`, "cron", "", "5", "2"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			f, h, p, tt := setup(t, 900)
			h.SetHandler(func(method string, raw json.RawMessage) (any, error) { if method == "chat.automations.template.apply" { return json.RawMessage(sample.raw), nil }; return f.handle(method, raw) })
			p.applyTemplate(Template{ID:"fixture"})
			for range 3 { h.Settle(tt) }
			d := p.draft
			if d == nil || d.timing != sample.timing || d.interval != sample.interval || d.scriptTimeout != sample.scriptTimeout || d.hookTimeout != sample.hookTimeout { t.Fatalf("visible template fields do not match their definition: %+v", d) }
			if !p.Dirty() || d.ID != "" || d.Value.WorkspaceKind != "dialogue" || d.Value.AgentKind != "pi" { t.Fatal("template hydration lost defaults or new-draft protection") }
		})
	}
}

func TestLateTemplateResultNeverReplacesANewerEditor(t *testing.T) {
	f, h, p, tt := setup(t, 900)
	entered, release := make(chan struct{}), make(chan struct{})
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.automations.template.apply" {
			close(entered)
			<-release
			return map[string]any{"name": "Template", "prompt": "Template instructions"}, nil
		}
		return f.handle(method, raw)
	})
	p.applyTemplate(Template{ID: "template"})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("template application never started")
	}
	p.openEditor(nil)
	p.draft.Value.Prompt = "Typed while applying"
	keep := p.draft
	close(release)
	h.Settle(tt)
	if p.draft != keep || p.draft.Value.Prompt != "Typed while applying" || p.templateErr == "" {
		t.Fatal("late template result silently overwrote newer work")
	}
}

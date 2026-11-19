package aiservices

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
	"winone/desktopmygo/i18n"
)

func TestPlatformPlanLoginRetainsProviderAndUsesExistingJob(t *testing.T) {
	f, h, p, _ := setup(t)
	pr := p.current()
	f.plan = map[string]any{"kind": "qianwen", "signedIn": false}
	e := p.editor
	e.plan.source = ""
	tt := ui.NewTester(func(c *ui.Context) { e.planView(c, pr, false) }, 1000, 1300)
	settle(h, tt)
	before := clone(*p.draft)
	shot(t, tt, "platform-plan-login")
	if err := tt.Click(planT("signIn", "name", "Qwen AI")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if f.signInParams["plan"] != true || f.signInParams["provider"] != pr.ID {
		t.Fatalf("platform owner lost: %v", f.signInParams)
	}
	if !equal(before, *p.draft) {
		t.Fatal("platform login changed provider configuration")
	}
	if e.plan.auth == nil || e.plan.auth.job == nil {
		t.Fatal("existing owned sign-in job not used")
	}
}

func TestAllowanceKeepsTextWindowsBalancesAndStaleReadTime(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	if got := allowanceWindowText(allowanceWindow{Name: "Monthly", Text: true, Display: "No included allowance"}); strings.Contains(got, "0%") || !strings.Contains(got, "No included allowance") {
		t.Fatal(got)
	}
	amount := 12.5
	if got := allowanceWindowText(allowanceWindow{Name: "Wallet", Balance: &amount, Unit: "credits"}); !strings.Contains(got, "12.5 credits") || strings.Contains(got, "0%") {
		t.Fatal(got)
	}
	f, h, p, _ := setup(t)
	stamp := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	f.quotas = []map[string]any{{"name": "Fixture", "plan": "Token Plan", "balance": "¥12.00", "asOf": stamp, "readError": "Vendor unavailable", "windows": []map[string]any{{"name": "Month", "used": 25, "resetsAt": stamp}}}}
	e := p.editor
	tt := ui.NewTester(func(c *ui.Context) { e.planView(c, p.current(), false) }, 1000, 1300)
	settle(h, tt)
	if err := tt.Click(planT("refresh")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	for _, label := range []string{planT("balance", "value", "¥12.00"), planT("cached", "time", stamp.Local().Format("2006-01-02 15:04")), "Vendor unavailable"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing allowance detail: %s", label)
		}
	}
	shot(t, tt, "platform-plan-allowance")
}

func TestAllowanceReadSurvivesIndependentSessionRefresh(t *testing.T) {
	_, h, p, tt := setup(t)
	e := p.editor
	// Hold the UI completion queue so the session generation changes while
	// an allowance read is already in progress.
	var pending []func()
	previous := p.env.Spawn
	p.env.Spawn = func(fn func()) { pending = append(pending, fn) }
	t.Cleanup(func() { p.env.Spawn = previous })
	e.readAllowance(p.current())
	e.loadPlan(p.current(), "get")
	for _, work := range pending {
		work()
	}
	p.env.Spawn = previous
	settle(h, tt)
	if e.plan.quotaBusy || !e.plan.quotaRead {
		t.Fatal("session status read left allowance loading forever")
	}
}

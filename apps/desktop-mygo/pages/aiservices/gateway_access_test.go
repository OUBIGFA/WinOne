package aiservices

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"winone/desktopmygo/i18n"
)

type keyStateTransport func(*http.Request) (*http.Response, error)

func (f keyStateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The editor reads live state on request and sends only a saved slot ID
// when retrying, without changing the configuration draft.
func TestNativeKeyRestReadAndRetryKeepTheProviderDraft(t *testing.T) {
	_, h, p, tt := setup(t)
	retried := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			if !strings.HasSuffix(r.URL.Path, "/api/winone/providers/one/key-rests/retry") {
				t.Error("retry reached another provider")
			}
			var in struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID != "primary" {
				t.Error("retry did not retain its slot", err)
			}
			retried = true
			fmt.Fprint(w, `{"keys":[]}`)
			return
		}
		fmt.Fprint(w, `{"keys":[{"id":"primary","rest":{"why":"credit","status":400,"until":"2099-01-01T01:00:00Z","error":"Vendor says credit balance is too low","by":"credit"}}]}`)
	})
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	http.DefaultTransport = keyStateTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Result(), nil
	})
	p.gateway.runtimeURL = "http://127.0.0.1:1/session/test/"
	pr := p.current()
	e := newEditor(p, pr, "fixture")
	defer e.Close()
	before := clone(*p.draft)
	tt = ui.NewTester(func(c *ui.Context) { e.keys(c, pr, false) }, 1100, 1600)
	settle(h, tt)
	if err := tt.Click(i18n.T("aiSync.readKeyState")); err != nil {
		t.Fatal("key-state read is not available", err)
	}
	settle(h, tt)
	if _, ok := tt.Find("Vendor says credit balance is too low"); !ok {
		t.Fatal("vendor rest explanation was dropped")
	}
	if err := tt.Click(i18n.T("aiSync.retryKeyNow")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !retried || !equal(before, p.draft) {
		t.Fatal("retry was not sent or changed the provider draft")
	}
}

// Unknown account resets and total budgets have no date to print. The
// actual production policy pane must retain their distinct instructions.
func TestGatewayBudgetDisplaysUnknownAndManualReset(t *testing.T) {
	_, h, p, tt := setup(t)
	a := &p.gateway.access
	var k callerKey
	if err := json.Unmarshal([]byte(`{"id":"fixture","name":"Fixture","limit":{"period":"account","account":"codex/user","accountWindow":"5h","tokens":1000},"usedAll":[{"tokens":1000,"cost":0,"period":"account","spent":true,"resetUnknown":true,"reset":"0001-01-01T00:00:00Z"}]}`), &k); err != nil {
		t.Fatal(err)
	}
	a.begin(p, k)
	a.tab = 2
	tt = ui.NewTester(func(c *ui.Context) { a.editorView(c, p) }, 1100, 1600)
	settle(h, tt)
	if _, ok := tt.Find(i18n.T("aiSync.accountResetUnknownSpent")); !ok {
		t.Fatal("unknown account reset was not explained in the policy pane")
	}
	a.edit.Limit.Period = "total"
	a.edit.UsedAll = nil
	settle(h, tt)
	if _, ok := tt.Find(i18n.T("aiSync.totalResetManual")); !ok {
		t.Fatal("total budget's manual reset is not explained")
	}
}

func TestNativeGatewayConcurrencyPolicyRoundTrip(t *testing.T) {
	p, h, tt, router := nativeAIWithRuntime(t)
	p.switchTab(tabGateway)
	p.gateway.tab = 1
	settle(h, tt)
	a := &p.gateway.access
	a.api.request(p, "POST", "/api/caller-keys/add-key", map[string]string{"name": "Concurrency fixture"}, func(raw json.RawMessage) { a.accept(raw) })
	settle(h, tt)
	if a.state == nil || len(a.state.Keys) != 1 {
		t.Fatal("create", a.api.err)
	}
	id := a.state.Keys[0].ID
	policy := map[string]any{"id": id, "name": "Concurrency fixture", "maxConcurrency": 2, "aliases": map[string]string{"daily": "fixture/model"}, "serviceTier": "priority"}
	a.api.request(p, "POST", "/api/winone/gateway/configure", policy, func(raw json.RawMessage) {
		var data struct {
			Keys []struct {
				MaxConcurrency int               `json:"maxConcurrency"`
				Aliases        map[string]string `json:"aliases"`
				ServiceTier    string            `json:"serviceTier"`
			}
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Keys) != 1 || data.Keys[0].MaxConcurrency != 2 || data.Keys[0].Aliases["daily"] != "fixture/model" || data.Keys[0].ServiceTier != "priority" {
			t.Errorf("concurrency was not saved: %s", raw)
		}
	})
	settle(h, tt)
	if a.api.err != "" {
		t.Fatal(a.api.err, router.URL)
	}
}

func TestNativeAccessKeysCreateEditScopesAndRecording(t *testing.T) {
	p, h, tt, _ := nativeAIWithRuntime(t)
	p.switchTab(tabGateway)
	p.gateway.tab = 1
	settle(h, tt)
	a := &p.gateway.access
	if a.state == nil || a.catalog == nil {
		t.Fatalf("access choices unavailable: %s", a.api.err)
	}
	if err := tt.Click(accessT("add")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	a.name = "Native test client"
	settle(h, tt)
	if err := tt.Click(i18n.T("common.save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if a.state == nil || len(a.state.Keys) != 1 || a.revealed == "" {
		t.Fatal("native create did not reach shared runtime")
	}
	a.begin(p, a.state.Keys[0])
	a.tab = 1
	settle(h, tt)
	a.edit.Models = []string{"fixture/model"}
	settle(h, tt)
	if err := tt.Click(i18n.T("common.save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(a.state.Keys[0].Models) != 1 || a.dirty() {
		t.Fatal("policy save did not round trip")
	}
	a.discard()
	a.revealed = ""
	p.gateway.historyOpen = true
	settle(h, tt)
	// Collapsible opens over 200 ms. Advance its test clock before scrolling:
	// while the panel is still clipped, the scroller cannot reach its controls.
	tt.Advance(200 * time.Millisecond)
	settle(h, tt)
	if bounds, ok := tt.Find(accessT("startRecording")); ok && bounds.Y+bounds.H > 1600 {
		tt.Scroll(550, 800, 0, bounds.Y-800)
		settle(h, tt)
	}
	if err := tt.Click(accessT("startRecording")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.gateway.history.state == nil || !p.gateway.history.state.Recording {
		t.Fatal("recording control did not reach runtime")
	}
	if os.Getenv("WINONE_SHOTS") != "" {
		defer i18n.SetLocale(i18n.Chinese)
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			i18n.SetLocale(locale)
			for _, tab := range []int{1, 2, 3} {
				p.gateway.tab = tab
				h.Shots(t, fmt.Sprintf("gateway-detail-%s-%d", locale, tab), p.View, 1200, func(tt *ui.Tester) { settle(h, tt) })
			}
			p.gateway.tab = 1
			a.begin(p, a.state.Keys[0])
			a.edit.Limit = &callerLimit{Period: "day", Tokens: 1000}
			for _, tab := range []int{0, 1, 2} {
				a.tab = tab
				h.Shots(t, fmt.Sprintf("gateway-policy-%s-%d", locale, tab), p.View, 1600, nil)
			}
			a.discard()
		}
	}
}

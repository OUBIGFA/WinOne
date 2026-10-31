package usage

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/widgets"
)

// fixture is a fake Host for the statistics: it answers the ledger,
// allowances, sessions, context and the exports from documents a test
// edits, and records every request through the real client and pipe.
type fixture struct {
	t     *testing.T
	h     *testhost.Harness
	u     *Usage
	tt    *ui.Tester
	opts  Options
	shown bool // whether the harness builds the tab this frame
	dark  bool

	mu      sync.Mutex
	ledger  func(query map[string]any) (any, error)
	quota   func(params map[string]any) (any, error)
	chat    func(method string, params map[string]any) (any, error)
	saves   func(method string, params map[string]any) (any, error)
	aliases func(params map[string]any) (any, error)
}

func ledgerFixture() map[string]any {
	totals := map[string]any{"calls": 2, "errors": 0, "input": 80, "output": 20, "cache_read": 0, "cache_write": 0, "cost": 0.02, "unpriced": 0}
	return map[string]any{"rows": []any{}, "sum": totals, "offset": 0, "limit": 15, "total": 0, "by": map[string]any{}}
}

func newFixture(t *testing.T, width, height int) *fixture {
	i18n.SetLocale(i18n.Chinese)
	f := &fixture{t: t, shown: true}
	f.ledger = func(map[string]any) (any, error) { return ledgerFixture(), nil }
	f.quota = func(map[string]any) (any, error) { return map[string]any{"quotas": []any{}}, nil }
	f.chat = func(method string, _ map[string]any) (any, error) {
		if method == "chat.catalog" {
			return map[string]any{"projects": nil, "conversations": []any{}}, nil
		}
		counts := map[string]any{"sessions": 0, "messages": 0, "calls": 0, "input": 0, "output": 0, "cache_read": 0, "cache_write": 0,
			"reasoning": 0, "errors": 0, "compactions": 0, "activeMs": 0, "cost": 0, "unpriced": 0}
		return map[string]any{"rows": nil, "sum": counts, "series": nil, "by": nil, "total": 0, "offset": 0, "limit": 15,
			"median": 0, "p90": 0, "top": nil, "tools": nil, "skills": nil, "warnings": nil, "shape": nil}, nil
	}
	f.h = testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var params map[string]any
		_ = json.Unmarshal(raw, &params)
		f.mu.Lock()
		ledger, quota, chat, saves, aliases := f.ledger, f.quota, f.chat, f.saves, f.aliases
		f.mu.Unlock()
		switch method {
		case "capture.ai.usage.get":
			return ledger(params)
		case "capture.ai.accounts.quota":
			return quota(params)
		case "chat.usage", "chat.catalog":
			return chat(method, params)
		case "capture.ai.usage.export.save", "capture.ai.archive.export.save", "capture.ai.archive.read":
			if saves != nil {
				return saves(method, params)
			}
		case "capture.ai.magpie.usage-aliases":
			if aliases != nil {
				return aliases(params)
			}
		}
		t.Errorf("unexpected %s %s", method, raw)
		return nil, testhost.Error{Code: "test", Message: "unexpected"}
	})
	f.u = New(f.h.Env)
	f.opts = Options{Active: true, Preferences: &Preferences{HideAccounts: true, QuotaLeft: true}}
	f.opts.Change = func(p Preferences) { f.opts.Preferences = &p }
	f.tt = ui.NewTester(f.view, width, height)
	f.settle()
	return f
}

func (f *fixture) view(c *ui.Context) {
	c.SetTheme(kit.Theme(c.Preferences(), f.dark))
	ui.Scroll(c).Fill().Padding(24, 20).Children(func() {
		if f.shown {
			f.u.View(c, f.opts)
		}
	})
}

// settle draws a frame for what changed outside the view, which may start
// reads, then waits for every read and applies it.
func (f *fixture) settle() {
	f.tt.Frame()
	f.h.Settle(f.tt)
}

func (f *fixture) click(label string) {
	f.t.Helper()
	if err := f.tt.Click(label); err != nil {
		f.t.Fatalf("%v; texts %q", err, f.tt.Texts())
	}
	f.settle()
}

// choose opens the drop-down of a field, below its label, and chooses
// option in it.
func (f *fixture) choose(label, option string) {
	f.t.Helper()
	box, ok := f.tt.Find(label)
	if !ok {
		f.t.Fatalf("no field %q in %q", label, f.tt.Texts())
	}
	f.tt.ClickAt(box.X+24, box.Y+box.H+6+16)
	f.settle()
	f.click(option)
}

// calls returns the parameters of the requests to method.
func (f *fixture) calls(method string) []map[string]any {
	var out []map[string]any
	for _, call := range f.h.Called(method) {
		var params map[string]any
		_ = json.Unmarshal(call.Params, &params)
		out = append(out, params)
	}
	return out
}

func (f *fixture) last(method string) map[string]any {
	f.t.Helper()
	calls := f.calls(method)
	if len(calls) == 0 {
		f.t.Fatalf("no %s", method)
	}
	return calls[len(calls)-1]
}

func (f *fixture) has(text string) bool { return f.tt.HasText(text) }

func (f *fixture) want(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.tt.HasText(text) {
			f.t.Fatalf("missing %q in %q", text, f.tt.Texts())
		}
	}
}

func requestRow(t, model string, extra map[string]any) map[string]any {
	r := map[string]any{"t": t, "provider": "one", "agent": "winone", "model": model, "in": 3, "out": 2, "ms": 1, "status": 200, "priced": false, "cost": 0,
		"cache_read": 0, "cache_write": 0}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func TestDefaultsToSevenDaysAndPagesFifteenRequests(t *testing.T) {
	f := newFixture(t, 1100, 4000)
	rows := make([]any, 15)
	for i := range rows {
		rows[i] = requestRow(time.Date(2026, 10, 10, 0, i, 0, 0, time.Local).Format(time.RFC3339), fmt.Sprint("model-", i), nil)
	}
	f.ledger = func(q map[string]any) (any, error) {
		page := ledgerFixture()
		page["total"], page["offset"] = 16, q["offset"]
		page["rows"] = rows
		if q["offset"].(float64) > 0 {
			page["rows"] = rows[:1]
		}
		return page, nil
	}
	f.u.revision++
	f.settle()
	first := f.calls("capture.ai.usage.get")[0]
	if first["period"] != "7d" || first["offset"] != 0.0 {
		t.Fatalf("first read %v", first)
	}
	if _, ok := first["tz"]; ok {
		t.Fatalf("the reader's zone is this computer's: %v", first)
	}
	f.click("请求明细")
	if !f.has("model-14") || !f.has("第 1–15 条，共 16 条") {
		t.Fatalf("first page %q", f.tt.Texts())
	}
	f.click("下一页")
	if f.last("capture.ai.usage.get")["offset"] != 15.0 {
		t.Fatalf("next page %v", f.last("capture.ai.usage.get"))
	}
	f.want("第 16–16 条，共 16 条")
}

func TestCSVExportSendsOnlyTheChosenPathAndConfirmsTheReceipt(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(q map[string]any) (any, error) {
		page := ledgerFixture()
		page["total"] = 16
		page["rows"] = []any{requestRow("2026-10-10T00:00:00Z", "fixture", nil)}
		return page, nil
	}
	receipt := map[string]any{"state": "saved", "path": `C:\Exports\chosen.csv`, "rows": 16}
	var mu sync.Mutex
	f.saves = func(method string, params map[string]any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		return receipt, nil
	}
	f.u.revision++
	f.settle()
	f.click("请求明细")
	// Cancelling the dialog sends nothing.
	f.click("导出 CSV")
	if len(f.calls("capture.ai.usage.export.save")) != 0 || f.has("已将") {
		t.Fatalf("a cancelled export reached the Host: %v", f.calls("capture.ai.usage.export.save"))
	}
	f.h.Picked["导出用量 CSV"] = `C:\Exports\chosen.csv`
	f.click("导出 CSV")
	saved := f.last("capture.ai.usage.export.save")
	query, _ := saved["query"].(map[string]any)
	if saved["path"] != `C:\Exports\chosen.csv` || query["period"] != "7d" || query["path"] != nil {
		t.Fatalf("export %v", saved)
	}
	f.want(`已将 16 条请求保存到 C:\Exports\chosen.csv`)
	// A name typed without its extension gets it.
	f.h.Picked["导出用量 CSV"] = `C:\Exports\chosen`
	f.click("导出 CSV")
	if f.last("capture.ai.usage.export.save")["path"] != `C:\Exports\chosen.csv` {
		t.Fatalf("extension %v", f.last("capture.ai.usage.export.save"))
	}
	// A receipt that confirms no file is no save.
	mu.Lock()
	receipt = map[string]any{"csv": "data without a saved file"}
	mu.Unlock()
	f.h.Picked["导出用量 CSV"] = `C:\Exports\other.csv`
	f.click("导出 CSV")
	f.want("无法确认 CSV 已保存")
	if f.has(`已将 16 条请求保存到 C:\Exports\other.csv`) {
		t.Fatal("an unconfirmed export was reported saved")
	}
}

func TestArchiveOpensOnDemandRetriesAndReportsOnlyCompletedExports(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	archive := "2026-10-26/101010-00000000000000aa"
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["total"] = 1
		page["rows"] = []any{requestRow("2026-10-26T00:00:00Z", "fixture", map[string]any{"archive": archive})}
		return page, nil
	}
	var mu sync.Mutex
	reads, exports := 0, 0
	f.saves = func(method string, params map[string]any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		switch method {
		case "capture.ai.archive.read":
			reads++
			if reads == 1 {
				return nil, testhost.Error{Code: "ai", Message: "归档读取失败"}
			}
			return map[string]any{"id": archive, "bytes": 20, "request": map[string]any{"body": "request fixture", "headers": map[string]any{}},
				"response": map[string]any{"body": "reply fixture", "headers": map[string]any{"Content-Type": []string{"application/json"}}}}, nil
		default:
			exports++
			if exports == 2 {
				return nil, testhost.Error{Code: "ai_export", Message: "归档保存失败"}
			}
			return map[string]any{"state": "saved", "path": params["path"]}, nil
		}
	}
	f.u.revision++
	f.settle()
	f.click("请求明细")
	f.click(dateTime("2026-10-26T00:00:00Z"))
	if reads != 0 {
		t.Fatal("bodies were read before asked")
	}
	f.click("查看请求归档")
	f.want("归档读取失败")
	f.click("重试")
	f.want("request fixture", "reply fixture", "请求头 / 响应头")
	f.click("导出归档 JSON")
	if exports != 0 {
		t.Fatal("a cancelled archive export reached the Host")
	}
	f.h.Picked["导出请求归档"] = `C:\Exports\archive.json`
	f.click("导出归档 JSON")
	f.want(`C:\Exports\archive.json`)
	if got := f.last("capture.ai.archive.export.save"); got["archive"] != archive || got["path"] != `C:\Exports\archive.json` || len(got) != 2 {
		t.Fatalf("archive export %v", got)
	}
	f.h.Picked["导出请求归档"] = `C:\Exports\second.json`
	f.click("导出归档 JSON")
	f.want("归档保存失败")
	f.click("重试")
	f.want(`C:\Exports\second.json`)
	if reads != 2 || exports != 3 {
		t.Fatalf("reads %d exports %d", reads, exports)
	}
}

func TestALateAnswerForAnOldFilterDoesNotReplaceTheCurrentOne(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	release := make(chan struct{})
	var mu sync.Mutex
	call := 0
	f.ledger = func(q map[string]any) (any, error) {
		mu.Lock()
		call++
		n := call
		mu.Unlock()
		if n == 1 {
			<-release
			return ledgerFixture(), nil
		}
		page := ledgerFixture()
		page["total"] = 1
		page["sum"] = map[string]any{"calls": 1, "errors": 1, "input": 0, "output": 0, "cache_read": 0, "cache_write": 0, "cost": 0, "unpriced": 0}
		return page, nil
	}
	// The newer read's answer is posted first, then the old one is let go.
	post := f.h.Env.Post
	posts := 0
	f.h.Env.Post = func(fn func()) {
		post(fn)
		posts++
		if posts == 1 {
			close(release)
		}
	}
	f.u.revision++
	f.tt.Frame()
	f.click("仅显示失败请求")
	if q := f.last("capture.ai.usage.get"); q["failed"] != true || q["offset"] != 0.0 {
		t.Fatalf("filtered read %v", q)
	}
	f.want("失败 · 1")
}

func TestInactiveReadsNothingAndADrillDownGivesBackItsFilters(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	f := newFixture(t, 1100, 3000)
	before := len(f.h.Calls())
	f.opts.Active = false
	f.tt.Frame()
	f.u.revision++
	f.settle()
	if len(f.h.Calls()) != before || len(f.tt.Texts()) != 0 {
		t.Fatalf("an inactive tab read or drew: %d %q", len(f.h.Calls())-before, f.tt.Texts())
	}
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["by"] = map[string]any{"model": []any{map[string]any{"id": "alpha", "calls": 1, "errors": 0, "input": 50, "output": 10, "cache_read": 0, "cache_write": 0, "cost": 0, "unpriced": 0}}}
		return page, nil
	}
	f.opts.Active = true
	f.settle()
	f.u.filter.Query = "kept"
	f.settle()
	// The overview's model ranking picks the model's requests.
	f.click("alpha")
	if f.u.view != viewRequests || f.u.filter.Model != "alpha" || f.last("capture.ai.usage.get")["model"] != "alpha" {
		t.Fatalf("drill-down %d %+v", f.u.view, f.u.filter)
	}
	// The page shows another tab for a frame: the filters come back.
	f.shown = false
	f.tt.Frame()
	f.shown = true
	f.settle()
	if f.u.filter.Model != "" || f.u.filter.Query != "kept" {
		t.Fatalf("filters after leaving %+v", f.u.filter)
	}
}

func TestReadFailureOffersRetry(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	var mu sync.Mutex
	fail := true
	f.ledger = func(map[string]any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			fail = false
			return nil, testhost.Error{Code: "ai", Message: "无法读取统计"}
		}
		return ledgerFixture(), nil
	}
	f.u.revision++
	f.settle()
	f.want("无法读取统计")
	f.click("重试")
	if f.has("无法读取统计") || !f.has("此范围内还没有请求记录。") {
		t.Fatalf("after retry %q", f.tt.Texts())
	}
}

func TestPriceWarningKeepsTotalsAndCreditsShowWithFirstTokenWait(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["priceWarning"] = true
		page["sum"] = map[string]any{"calls": 2, "errors": 0, "input": 40, "output": 10, "cache_read": 0, "cache_write": 0, "cost": 0, "unpriced": 2, "credits": 0, "credited": 1}
		page["total"] = 1
		page["rows"] = []any{requestRow("2026-10-17T00:00:00Z", "fixture", map[string]any{"ms": 1500, "ttft_ms": 125, "credits": 0})}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.want("模型价格更新失败。已有价格继续用于估算，未知价格不计入费用。请刷新重试。", "40", "10", "套餐积分", "2 个请求没有价格信息，未计入估算费用。")
	f.click("请求明细")
	f.want("首字延迟", "0.13 s", "0 套餐积分", "1.50 s")
}

func TestRequestsExplainMissingSpeedsAndLabelLowerBounds(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	base := map[string]any{"in": 3, "out": 1000, "ms": 2000}
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["total"] = 2
		page["rows"] = []any{requestRow("2026-10-23T00:00:00Z", "fixture", base), requestRow("2026-10-23T00:01:00Z", "fixture", map[string]any{"in": 3, "out": 1000, "ms": 2000, "status": 500})}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.click("请求明细")
	f.want("≥500 Token/s", "失败 500")
}

func TestGatewayKeyFilterIncludesCallsWithoutAKey(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["callerKeys"] = []any{map[string]any{"id": "public-hash", "callerKeyId": "public-hash", "callerKeyName": "Fixture gateway"}}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.click("请求明细")
	f.choose("网关密钥", "Fixture gateway")
	if q := f.last("capture.ai.usage.get"); q["callerKey"] != "public-hash" || q["offset"] != 0.0 {
		t.Fatalf("key filter %v", q)
	}
	f.choose("网关密钥", "未使用网关密钥")
	if q := f.last("capture.ai.usage.get"); q["callerKey"] != "none" {
		t.Fatalf("no key filter %v", q)
	}
}

func TestChosenDatesApplyAsOneInclusiveRange(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	f.choose("时间范围", "自选日期")
	if !f.has("起始日期") || !f.has("应用日期") {
		t.Fatalf("date fields %q", f.tt.Texts())
	}
	// Reversed dates are put in order.
	f.u.period.from = time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local)
	f.u.period.to = time.Date(2026, 10, 8, 0, 0, 0, 0, time.Local)
	f.click("应用日期")
	if q := f.last("capture.ai.usage.get"); q["period"] != "2026-10-08..2026-10-09" || q["offset"] != 0.0 {
		t.Fatalf("range %v", q)
	}
}

func TestViewTabsAskTheGuard(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	requested := 0
	f.h.Env.RequestChange = func(apply func()) { requested++ }
	f.click("会话")
	if requested != 1 || f.u.view != viewOverview {
		t.Fatalf("switched without the guard: %d %d", requested, f.u.view)
	}
	f.h.Env.RequestChange = func(apply func()) { apply() }
	f.click("会话")
	if f.u.view != viewSessions {
		t.Fatalf("view %d", f.u.view)
	}
}

func TestAutomaticRefreshCountsOnlyWhileShown(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	reads := len(f.calls("capture.ai.usage.get"))
	f.choose("自动刷新", "每 15 秒")
	f.tt.Advance(14 * time.Second)
	f.settle()
	if n := len(f.calls("capture.ai.usage.get")); n != reads {
		t.Fatalf("refreshed early: %d", n-reads)
	}
	f.tt.Advance(2 * time.Second)
	f.settle()
	if n := len(f.calls("capture.ai.usage.get")); n != reads+1 {
		t.Fatalf("refreshes %d", n-reads)
	}
	f.opts.Active = false
	f.tt.Frame()
	f.tt.Advance(time.Minute)
	f.settle()
	if n := len(f.calls("capture.ai.usage.get")); n != reads+1 {
		t.Fatalf("an inactive tab refreshed: %d", n-reads)
	}
}

func TestAliasesValidateTheDraftAndLeaveSavingToThePage(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	f.opts.Preferences.Aliases = map[string]string{"old": "new"}
	var mu sync.Mutex
	reply := func(map[string]any) (any, error) {
		return map[string]any{"usageAliases": map[string]any{"old": "new", "new-model": "combined-model"}}, nil
	}
	f.aliases = func(p map[string]any) (any, error) { mu.Lock(); defer mu.Unlock(); return reply(p) }
	f.click("模型归并")
	f.click("原模型标识")
	f.tt.Type(" new-model")
	f.click("归并名称")
	f.tt.Type("combined-model")
	f.settle()
	if dirty, _ := f.h.Env.Activity.State(); !dirty {
		t.Fatal("unapplied input is not reported")
	}
	f.click("应用归并")
	call := f.last("capture.ai.magpie.usage-aliases")
	edit, _ := call["edit"].(map[string]any)
	if edit["from"] != " new-model" || edit["to"] != "combined-model" || call["aliases"].(map[string]any)["old"] != "new" {
		t.Fatalf("validation %v", call)
	}
	if got := f.opts.Preferences.Aliases; got["new-model"] != "combined-model" || f.u.aliases.from != "" {
		t.Fatalf("draft %v form %q", got, f.u.aliases.from)
	}
	if dirty, busy := f.h.Env.Activity.State(); dirty || busy {
		t.Fatal("an applied form still counts as unsaved")
	}
	f.want("添加或移除后，点击“保存”生效。", "combined-model")
	// A failed check keeps the form and the draft.
	mu.Lock()
	reply = func(map[string]any) (any, error) {
		return nil, testhost.Error{Code: "ai", Message: "归并关系形成循环"}
	}
	mu.Unlock()
	f.u.aliases.from, f.u.aliases.to = "new", "old"
	f.settle()
	f.click("应用归并")
	f.want("归并关系形成循环")
	if f.u.aliases.from != "new" || len(f.opts.Preferences.Aliases) != 2 {
		t.Fatalf("after a failed check: %q %v", f.u.aliases.from, f.opts.Preferences.Aliases)
	}
	// Discarding the page's changes clears the form.
	f.h.Env.Activity.Discard()
	if f.u.aliases.from != "" {
		t.Fatal("discard kept the form")
	}
	// Removing goes through the same check; a locked page sends nothing.
	mu.Lock()
	reply = func(map[string]any) (any, error) { return map[string]any{"usageAliases": map[string]any{}}, nil }
	mu.Unlock()
	calls := len(f.calls("capture.ai.magpie.usage-aliases"))
	f.opts.Disabled = true
	f.settle()
	f.click("移除 old 的归并")
	if len(f.calls("capture.ai.magpie.usage-aliases")) != calls {
		t.Fatal("a locked page sent an edit")
	}
	f.opts.Disabled = false
	f.settle()
	f.click("移除 old 的归并")
	if edit, _ := f.last("capture.ai.magpie.usage-aliases")["edit"].(map[string]any); edit["from"] != "old" || edit["to"] != "" {
		t.Fatalf("removal %v", edit)
	}
	if len(f.opts.Preferences.Aliases) != 0 {
		t.Fatalf("draft after removal %v", f.opts.Preferences.Aliases)
	}
}

func TestAliasesHideWithoutADraft(t *testing.T) {
	f := newFixture(t, 1100, 2000)
	f.opts.Change = nil
	f.settle()
	if f.has("模型归并") {
		t.Fatal("aliases offered without a draft")
	}
}

func providers() []Provider {
	return []Provider{
		{AIProvider: widgets.AIProvider{ID: "saved-account", Name: "Configured account"}, AuthMode: "account", Agent: "codex"},
	}
}

func TestAllowancesUseTheSavedChoicesAndNeverInventAReading(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.opts.Providers = providers()
	f.opts.Preferences = &Preferences{QuotaLeft: false, HideAccounts: false}
	f.quota = func(p map[string]any) (any, error) {
		return map[string]any{"quotas": []any{map[string]any{"provider": "codex", "name": "Codex", "user": "fixture@example.invalid",
			"windows": []any{map[string]any{"name": "Five hours", "used": 25}}}}}, nil
	}
	f.u.revision++
	f.settle()
	f.want("25.0% 已用", "fixture@example.invalid")
	if q := f.last("capture.ai.accounts.quota"); q["provider"] != "saved-account" || q["agent"] != "codex" {
		t.Fatalf("quota read %v", q)
	}
	f.click("显示剩余")
	if f.opts.Preferences.QuotaLeft != true || !f.has("75.0% 剩余") {
		t.Fatalf("left %v %q", f.opts.Preferences, f.tt.Texts())
	}
	f.click("隐藏账号")
	if !f.opts.Preferences.HideAccounts || f.has("fixture@example.invalid") || !f.has("账号 1") {
		t.Fatalf("masked %q", f.tt.Texts())
	}
	f.quota = func(map[string]any) (any, error) {
		return nil, testhost.Error{Code: "ai", Message: "额度接口离线"}
	}
	f.click("刷新统计")
	f.want("额度接口离线")
	if f.has("75.0% 剩余") {
		t.Fatal("a failed read kept an old allowance")
	}
}

func TestAStaleAllowanceIsAFailedRefreshAndConsolesAreWebOnly(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.opts.Providers = []Provider{{AIProvider: widgets.AIProvider{ID: "zen", Name: "OpenCode Zen"}, AuthMode: "api-key", HasKey: true}}
	f.quota = func(map[string]any) (any, error) {
		return map[string]any{"quotas": []any{
			map[string]any{"provider": "codex", "name": "Codex", "asOf": "2026-10-10T10:00:00Z", "windows": []any{map[string]any{"name": "Five hours", "used": 25}}},
			map[string]any{"provider": "zen", "name": "OpenCode Zen", "windows": []any{}, "error": "OpenCode tells no API key its balance", "seeAt": "javascript:alert(1)"},
			map[string]any{"provider": "zen", "name": "OpenCode Zen", "user": "b", "windows": []any{}, "error": "OpenCode tells no API key its balance", "seeAt": "https://opencode.ai/console"},
		}}, nil
	}
	f.u.revision++
	f.settle()
	f.want("75.0% 剩余", "OpenCode 不通过 API 提供余额")
	if !strings.Contains(strings.Join(f.tt.Texts(), "\n"), "本次额度读取失败，以下为") {
		t.Fatalf("stale reading %q", f.tt.Texts())
	}
	if _, ok := f.tt.Find("打开控制台"); !ok {
		t.Fatal("no console link")
	}
	f.click("打开控制台")
	if opened := f.tt.OpenedURLs(); len(opened) != 1 || opened[0] != "https://opencode.ai/console" {
		t.Fatalf("opened %q", opened)
	}
	if q := f.last("capture.ai.accounts.quota"); q["provider"] != "zen" || q["agent"] != nil {
		t.Fatalf("key balance read %v", q)
	}
}

func TestOneAllowanceReadInFlightServesARefresh(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	reads := 0
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if method != "capture.ai.accounts.quota" {
			t.Errorf("unexpected %s", method)
		}
		mu.Lock()
		reads++
		wait := release
		mu.Unlock()
		<-wait
		return map[string]any{"quotas": []any{map[string]any{"provider": "codex", "name": "Codex", "windows": []any{map[string]any{"name": "Five hours", "used": 25}}}}}, nil
	})
	a := newAllowances(h.Env)
	list := providers()
	a.read(list)
	// A refresh while the vendor is still read joins that read.
	a.read(list)
	if !a.loading {
		t.Fatal("no loading state while reading")
	}
	close(release)
	h.Settle(ui.NewTester(func(*ui.Context) {}, 10, 10))
	mu.Lock()
	defer mu.Unlock()
	if reads != 1 || a.loading || len(a.data) != 1 || len(a.data[0].quotas) != 1 || a.data[0].err != "" {
		t.Fatalf("reads %d data %+v", reads, a.data)
	}
	// A refresh after it ended reads anew.
	mu.Unlock()
	a.read(list)
	h.Settle(ui.NewTester(func(*ui.Context) {}, 10, 10))
	mu.Lock()
	if reads != 2 {
		t.Fatalf("reads after the first ended %d", reads)
	}
}

func TestSessionsAndContextUseWinOnesOwnRecords(t *testing.T) {
	f := newFixture(t, 1100, 4000)
	prompt := map[string]any{"window": 1000, "tokens": 100, "counted": true, "parts": []any{
		map[string]any{"kind": "system", "tokens": 40, "items": []any{map[string]any{"name": "prompt", "tokens": 40}}},
		map[string]any{"kind": "chat", "tokens": 60, "items": []any{map[string]any{"name": "1", "tag": "turn", "tokens": 60}}},
	}}
	counts := map[string]any{"sessions": 1, "messages": 2, "calls": 1, "input": 100, "output": 20, "cache_read": 0, "cache_write": 0,
		"reasoning": 0, "errors": 0, "compactions": 0, "activeMs": 60000, "cost": 0.02, "unpriced": 0}
	session := map[string]any{"id": "registered", "title": "My chat", "provider": "fixture", "model": "model", "project": "",
		"createdAt": "2026-10-10T01:00:00Z", "updatedAt": "2026-10-10T01:00:01Z", "archived": false,
		"context": []any{map[string]any{"time": "2026-10-10T01:00:00Z", "input": 100, "cache_read": 0, "output": 20}}}
	for k, v := range counts {
		session[k] = v
	}
	f.chat = func(method string, _ map[string]any) (any, error) {
		if method == "chat.catalog" {
			return map[string]any{"projects": nil, "conversations": []any{map[string]any{"id": "registered", "title": "My chat"}}}, nil
		}
		return map[string]any{"rows": []any{session}, "sum": counts, "series": []any{map[string]any{"time": "2026-10-10", "sessions": 1, "messages": 2,
			"calls": 1, "input": 100, "output": 20, "cache_read": 0, "cache_write": 0, "reasoning": 0, "errors": 0, "compactions": 0, "activeMs": 60000, "cost": 0.02, "unpriced": 0}},
			"by": map[string]any{"model": map[string]any{"model": counts}}, "total": 1, "offset": 0, "limit": 15, "median": 120, "p90": 120,
			"top": []any{}, "tools": map[string]any{"read": 1}, "skills": map[string]any{}, "warnings": []any{},
			"shape": map[string]any{"messages": map[string]any{"edges": []int{1, 6, 16, 31, 61, 121}, "counts": []int{1, 0, 0, 0, 0, 0}, "total": 1}}}, nil
	}
	f.ledger = func(q map[string]any) (any, error) {
		page := ledgerFixture()
		page["context"] = map[string]any{"agents": []any{}, "sessions": []any{map[string]any{"agent": "fixture", "key": "registered", "model": "model",
			"requests": 1, "peak": 100, "window": 1000, "latest": prompt, "latestId": 1, "points": []any{map[string]any{"id": 1, "time": "2026-10-10T01:00:00Z", "tokens": 100}}}}}
		page["contextTotal"] = 1
		page["contextRequests"] = []any{map[string]any{"id": 1, "time": "2026-10-10T01:00:00Z", "agent": "fixture", "session": "registered", "model": "model", "prompt": prompt}}
		return page, nil
	}
	f.click("上下文")
	if q := f.last("capture.ai.usage.get"); q["period"] != "7d" || q["provider"] != "" || q["offset"] != 0.0 {
		t.Fatalf("context read %v", q)
	}
	f.click("My chat")
	f.want("系统提示 · 40 Token", "对话 · 60 Token", "上下文窗口 · 100 / 1000 Token", "打开会话", "尚无上下文记录。发送新的 AI 请求后可在这里查看。")
	if _, ok := f.tt.Find("上下文组成"); !ok {
		t.Fatal("no context grid")
	}
	f.click("会话")
	f.want("最近会话", "$0.020000", "My chat", "read")
	if q := f.last("chat.usage"); q["period"] != "7d" || q["offset"] != 0.0 || q["topBy"] != "tokens" {
		t.Fatalf("sessions read %v", q)
	}
	f.want("全部项目")
}

func TestEmptyCatalogKeepsTheAllProjectsFilter(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.click("会话")
	f.want("此范围内还没有会话。", "全部项目")
}

func TestOverviewRankingsFoldToTheirTopSeven(t *testing.T) {
	f := newFixture(t, 1100, 4000)
	accounts := make([]any, 9)
	for i := range accounts {
		accounts[i] = map[string]any{"id": fmt.Sprint("account-", i), "calls": 1, "errors": 0, "input": 100 - i, "output": 20, "cache_read": 0, "cache_write": 0, "cost": 0.02, "unpriced": 0}
	}
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["by"] = map[string]any{"account": accounts}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.want("account-8", "收起为前 7 项")
	f.click("收起为前 7 项")
	f.want("其他（2）", "查看全部 9 项")
	if f.has("account-8") {
		t.Fatal("a folded ranking still lists every account")
	}
	f.click("查看全部 9 项")
	f.click("account-8")
	if f.u.view != viewRequests || f.u.filter.Account != "account-8" {
		t.Fatalf("account pick %+v", f.u.filter)
	}
}

func TestChartStyleAndMetricAreRemembered(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["bucket"] = "day"
		page["series"] = []any{map[string]any{"time": "2026-10-20T00:00:00+08:00", "calls": 2, "errors": 0, "input": 80, "output": 20, "cache_read": 0, "cache_write": 0, "cost": 0.02, "unpriced": 0}}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.click("折线图")
	// The metric's options repeat the totals' names: choose by keys,
	// from the tokens to the cost below them.
	box, _ := f.tt.Find("统计指标")
	f.tt.ClickAt(box.X+24, box.Y+box.H+6+16)
	f.tt.Key(0, ui.KeyDown)
	f.tt.Key(0, ui.KeyEnter)
	f.settle()
	again := New(f.h.Env)
	if again.prefs.Chart != "line" || again.prefs.Metric != "cost" {
		t.Fatalf("remembered %+v", again.prefs)
	}
}

func TestChartDaysPickTheirRequests(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(q map[string]any) (any, error) {
		page := ledgerFixture()
		page["bucket"] = "day"
		page["day"] = q["day"]
		var series []any
		for i := range 3 {
			series = append(series, map[string]any{"time": fmt.Sprintf("2026-10-%02dT00:00:00+08:00", 20+i), "calls": 2, "errors": 0, "input": 80,
				"output": 20, "cache_read": 0, "cache_write": 0, "cost": 0.02, "unpriced": 0})
		}
		page["series"] = series
		return page, nil
	}
	f.u.revision++
	f.settle()
	day := ledWhen(point{Time: "2026-10-21T00:00:00+08:00"}, "day")
	f.click(day)
	if f.u.filter.Day != "2026-10-21" || f.last("capture.ai.usage.get")["day"] != "2026-10-21" {
		t.Fatalf("day %+v", f.u.filter)
	}
	f.click(day)
	if f.u.filter.Day != "" {
		t.Fatalf("a second click keeps the day %+v", f.u.filter)
	}
}

func TestLeavingTheRequestsGivesBackTheFiltersADrillDownReplaced(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["by"] = map[string]any{"provider": []any{map[string]any{"id": "openai", "calls": 1, "errors": 0, "input": 50, "output": 10, "cache_read": 0, "cache_write": 0, "cost": 0, "unpriced": 0}}}
		return page, nil
	}
	f.u.change(func(q *filter) { q.Failed = true })
	f.settle()
	f.u.pick("modelAt", "openai/gpt-5.5")
	f.settle()
	if q := f.last("capture.ai.usage.get"); q["provider"] != "openai" || q["model"] != "gpt-5.5" || q["failed"] != true {
		t.Fatalf("drill-down %v", q)
	}
	f.click("概览")
	if f.u.filter.Provider != "" || f.u.filter.Model != "" || !f.u.filter.Failed || f.u.origin != nil {
		t.Fatalf("filters after leaving %+v", f.u.filter)
	}
}

func TestLeavingTheAliasesForgetsTheirForm(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	f.click("模型归并")
	f.click("原模型标识")
	f.tt.Type("draft")
	f.settle()
	if dirty, _ := f.h.Env.Activity.State(); !dirty {
		t.Fatal("unapplied input is not reported")
	}
	f.click("概览")
	if dirty, _ := f.h.Env.Activity.State(); dirty || f.u.aliases.from != "" {
		t.Fatalf("form after leaving %q", f.u.aliases.from)
	}
}

func TestClosingForgetsTheFormAndDropsLateAnswers(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	f.click("模型归并")
	f.click("原模型标识")
	f.tt.Type("draft")
	f.settle()
	f.u.revision++
	f.u.view = viewOverview
	f.tt.Frame()
	f.u.Close()
	f.h.Settle(f.tt)
	if dirty, busy := f.h.Env.Activity.State(); dirty || busy {
		t.Fatal("a closed tab still counts")
	}
}

func TestHeatmapMetricIsRemembered(t *testing.T) {
	f := newFixture(t, 1100, 3000)
	f.ledger = func(map[string]any) (any, error) {
		page := ledgerFixture()
		page["heatmap"] = map[string]any{"from": "2026-10-05", "to": "2026-10-11", "days": []any{map[string]any{"date": "2026-10-11", "calls": 2, "tokens": 1250, "cost": 0.03}}}
		return page, nil
	}
	f.u.revision++
	f.settle()
	f.click("请求明细")
	f.want("请求活动热图", "最近 53 周，按当前筛选条件")
	day, _ := f.tt.Find(formatDate(time.Date(2026, 10, 11, 0, 0, 0, 0, time.Local), "yMMMd"))
	f.tt.Move(day.X+day.W/2, day.Y+day.H/2)
	f.settle()
	f.want("1250 Token · 2 次请求 · ≈$0.030")
	// "请求数" also names a total: choose the heatmap's metric by its keys.
	f.click("Token")
	f.tt.Key(0, ui.KeyRight)
	f.settle()
	f.tt.Move(day.X+day.W/2, day.Y+day.H/2)
	f.settle()
	f.want("2 次请求 · 1250 Token · ≈$0.030")
	if again := New(f.h.Env); again.prefs.Heatmap != "calls" {
		t.Fatalf("heatmap metric %q", again.prefs.Heatmap)
	}
}

func TestASavedConfigurationReadsAgainAndPicksAreSaid(t *testing.T) {
	f := newFixture(t, 1100, 2400)
	f.opts.Providers = []Provider{{AIProvider: widgets.AIProvider{ID: "openai", Name: "OpenAI", Models: []string{"gpt-5.5"}, ModelNames: map[string]string{"gpt-5.5": "GPT-5.5"}}}}
	reads := len(f.calls("capture.ai.usage.get"))
	f.opts.Revision = 7
	f.settle()
	if n := len(f.calls("capture.ai.usage.get")); n != reads+1 {
		t.Fatalf("a new revision read %d times", n-reads)
	}
	f.u.change(func(q *filter) { q.Day, q.Model = "2026-10-21", "gpt-5.5" })
	f.settle()
	f.want("2026-10-21 GPT-5.5")
}

func TestSessionDetailsShowTheirCountsAndContext(t *testing.T) {
	f := richFixture(t, 1100, 6000)
	f.click("会话")
	f.click("Heatmap layout")
	f.want("打开会话", "上下文记录", "30000", "20000")
	navigated := ""
	f.h.Env.Navigate = func(page string, _ func()) { navigated = page }
	f.click("打开会话")
	if navigated != "chat" {
		t.Fatalf("open chat went to %q", navigated)
	}
}

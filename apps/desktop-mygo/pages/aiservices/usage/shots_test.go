package usage

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/widgets"
)

// richFixture fills a fixture with a week of requests by several models
// and providers, a year's heatmap, allowances with their history, Chat
// sessions and the context analysis, for the layout checks and the
// screenshots.
func richFixture(t *testing.T, width, height int) *fixture {
	f := newFixture(t, width, height)
	now := time.Now()
	day := func(i int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, i-6)
	}
	models := []string{"gpt-5.5", "claude-sonnet-4.5", "deepseek-v4.1-flash", "qwen3-coder-plus", "gemini-3.1-pro", "kimi-k2.5", "glm-5", "mimo-v2.5-pro", "minimax-m2"}
	f.opts.Providers = []Provider{
		{AIProvider: widgets.AIProvider{ID: "openai", Name: "OpenAI", Icon: "openai", Models: models[:3], ModelNames: map[string]string{"gpt-5.5": "GPT-5.5"}}, AuthMode: "api-key", HasKey: true},
		{AIProvider: widgets.AIProvider{ID: "codex", Name: "Codex 账号", Icon: "openai", Models: models[3:]}, AuthMode: "account", Agent: "codex"},
	}
	share := func(id string, scale float64) map[string]any {
		return map[string]any{"id": id, "calls": 40 * scale, "errors": float64(int(scale * 3)), "input": 120000 * scale, "output": 18000 * scale,
			"cache_read": 60000 * scale, "cache_write": 4000 * scale, "cost": 1.234 * scale, "unpriced": 0, "timed": 30 * scale,
			"ttft_ms": 24000 * scale, "decode_ms": 60000 * scale, "decode_out": 9000 * scale, "decode_n": 30 * scale}
	}
	var byModel, byModelAt []any
	for i, m := range models {
		scale := 1.0 / float64(i+1)
		byModel = append(byModel, share(m, scale))
		provider := "openai"
		if i >= 3 {
			provider = "codex"
		}
		byModelAt = append(byModelAt, share(provider+"/"+m, scale))
	}
	var series []any
	for i := range 7 {
		parts := map[string]any{}
		for k, m := range models[:7] {
			v := float64((i+k)%5+1) / float64(k+1)
			parts[m] = map[string]any{"calls": 6 * v, "tokens": 30000 * v, "cost": 0.2 * v, "timed": 4 * v, "ttft_ms": 3000 * v, "decode_ms": 9000 * v, "decode_out": 900 * v}
		}
		series = append(series, map[string]any{"time": day(i).Format(time.RFC3339), "calls": 60, "errors": i % 2, "input": 180000, "output": 26000,
			"cache_read": 70000, "cache_write": 5000, "cost": 2.1, "unpriced": 0, "timed": 40, "ttft_ms": 30000, "decode_ms": 80000, "decode_out": 12000,
			"by": map[string]any{"model": parts}})
	}
	var days []any
	for i := 0; i < 371; i += 2 {
		d := day(6).AddDate(0, 0, -i)
		days = append(days, map[string]any{"date": d.Format(time.DateOnly), "calls": float64((i*7)%23 + 1), "tokens": float64((i*1931)%90000 + 1000), "cost": float64(i%9) / 10})
	}
	from := day(6)
	from = from.AddDate(0, 0, -(int(from.Weekday())+6)%7-7*52)
	var rows []any
	for i := range 6 {
		extra := map[string]any{"in": 1200 + i*300, "out": 900 + i*80, "ms": 2400 + i*500, "ttft_ms": 600 + i*40, "priced": i%3 != 2, "cost": 0.0012 * float64(i+1),
			"cache_read": 4000, "cache_write": 200, "req": models[i], "served": models[i], "ep": "/v1/chat/completions", "effort": "high", "stop": "stop"}
		if i == 2 {
			extra["status"], extra["err"] = 429, "Request failed"
		}
		if i == 1 {
			extra["archive"] = "2026-10-26/101010-00000000000000aa"
			extra["credits"] = 0.5
		}
		r := requestRow(day(6).Add(time.Duration(9+i)*time.Hour).Format(time.RFC3339), models[i], extra)
		if i >= 3 {
			r["provider"], r["agent"] = "codex", "gateway"
		}
		rows = append(rows, r)
	}
	prompt := map[string]any{"window": 200000, "tokens": 48000, "counted": true, "parts": []any{
		map[string]any{"kind": "system", "tokens": 9000, "items": []any{map[string]any{"name": "prompt", "tokens": 9000}}},
		map[string]any{"kind": "tools", "tokens": 12000, "items": []any{map[string]any{"name": "read_file", "tokens": 7000}, map[string]any{"name": "github", "tag": "mcp", "tokens": 5000}}},
		map[string]any{"kind": "memory", "tokens": 4000, "items": []any{map[string]any{"name": "C:/Users/fixture/AGENTS.md", "tag": "project", "tokens": 4000}}},
		map[string]any{"kind": "files", "tokens": 8000, "items": []any{map[string]any{"name": "apps/desktop-mygo/pages/aiservices/usage/ledger.go", "tokens": 8000}}},
		map[string]any{"kind": "results", "tokens": 5000},
		map[string]any{"kind": "chat", "tokens": 10000, "items": []any{map[string]any{"name": "12", "tag": "turn", "tokens": 10000}}},
	}}
	var points []any
	for i := range 24 {
		points = append(points, map[string]any{"id": i + 1, "time": day(5).Add(time.Duration(i) * time.Minute).Format(time.RFC3339), "tokens": 12000 + i*1600})
	}
	f.ledger = func(q map[string]any) (any, error) {
		page := map[string]any{
			"rows": rows, "total": 46, "offset": q["offset"], "limit": 15, "bucket": "day", "day": q["day"], "series": series,
			"sum": map[string]any{"calls": 420, "errors": 7, "input": 1260000, "output": 182000, "cache_read": 490000, "cache_write": 35000, "cost": 14.70321,
				"unpriced": 3, "credits": 12.5, "credited": 9, "timed": 280, "ttft_ms": 210000, "decode_ms": 560000, "decode_out": 84000, "decode_n": 280,
				"floor_out": 30000, "floor_ms": 200000, "floor_n": 12, "cache_unknown": 20000, "cache_unknown_calls": 4},
			"by": map[string]any{"model": byModel, "modelAt": byModelAt, "provider": []any{share("openai", 1), share("codex", 0.6)},
				"agent": []any{share("winone", 1), share("gateway", 0.4)}, "account": []any{share("codex", 0.6)}},
			"heatmap":    map[string]any{"from": from.Format(time.DateOnly), "to": day(6).Format(time.DateOnly), "days": days},
			"callerKeys": []any{map[string]any{"id": "k1", "callerKeyName": "Fixture gateway"}},
			"accounts":   []any{map[string]any{"id": "codex/a", "account": "codex", "name": "Codex"}},
			"purposes":   []any{"translate", "chat"},
			"context": map[string]any{"agents": []any{map[string]any{"agent": "openai", "requests": 120, "sessions": 9, "median": 30000, "p90": 61000,
				"baseline": 21000, "windowKnown": true, "cache": 0.62, "growth": 1600, "compacts": 2, "score": 72,
				"shares": map[string]any{"system": 0.2, "tools": 0.25, "memory": 0.08, "files": 0.17, "results": 0.1, "chat": 0.2},
				"scores": []any{map[string]any{"key": "cache", "points": 22, "most": 30}, map[string]any{"key": "lean", "points": 18, "most": 25},
					map[string]any{"key": "pace", "points": 17, "most": 25}, map[string]any{"key": "reliable", "points": 15, "most": 20}},
				"tags": []any{map[string]any{"key": "cache-friendly", "tone": "good"}, map[string]any{"key": "tool-heavy", "tone": "info", "value": 0.25}}}},
				"sessions": []any{map[string]any{"agent": "openai", "key": "registered", "model": "gpt-5.5", "requests": 24, "peak": 48800, "window": 200000,
					"compacts": 1, "latest": prompt, "latestId": 24, "points": points}}},
			"contextTotal":    1,
			"contextRequests": []any{map[string]any{"id": 24, "session": "registered", "prompt": prompt}},
		}
		return page, nil
	}
	f.quota = func(p map[string]any) (any, error) {
		if p["provider"] == "openai" {
			return map[string]any{"quotas": []any{map[string]any{"provider": "openai", "name": "OpenAI", "windows": []any{}, "balance": "$42.10",
				"balanceSplit": []any{map[string]any{"label": "Granted", "text": "$2.10"}, map[string]any{"label": "Topped up", "text": "$40.00"}},
				"readAt":       now.Add(-time.Minute).UTC().Format(time.RFC3339)}}}, nil
		}
		start := now.Add(-3 * time.Hour)
		reset := now.Add(2 * time.Hour)
		var history []any
		for i := range 8 {
			at := start.Add(time.Duration(i) * 20 * time.Minute)
			history = append(history, map[string]any{"at": at.UTC().Format(time.RFC3339), "left": 100 - float64(i)*7, "start": start.UTC().Format(time.RFC3339), "resetsAt": reset.UTC().Format(time.RFC3339)})
		}
		return map[string]any{"quotas": []any{map[string]any{"provider": "codex", "name": "Codex", "user": "fixture@example.invalid", "plan": "Pro",
			"windows": []any{map[string]any{"name": "5 hours", "used": 49, "resetsAt": reset.UTC().Format(time.RFC3339), "display": "49% used"},
				map[string]any{"name": "Week", "used": 12}},
			"history": []any{map[string]any{"name": "5 hours", "points": history}}, "readAt": now.UTC().Format(time.RFC3339)}}}, nil
	}
	f.chat = func(method string, _ map[string]any) (any, error) {
		if method == "chat.catalog" {
			return map[string]any{"projects": []any{map[string]any{"id": "p1", "name": "WinOne"}}, "conversations": []any{map[string]any{"id": "registered", "title": "Usage port review"}}}, nil
		}
		counts := func(scale float64) map[string]any {
			return map[string]any{"sessions": 1 * scale, "messages": 30 * scale, "calls": 14 * scale, "input": 90000 * scale, "output": 12000 * scale, "cache_read": 40000 * scale,
				"cache_write": 2000 * scale, "reasoning": 3000 * scale, "errors": 0, "compactions": 1, "activeMs": 1800000 * scale, "cost": 0.42 * scale, "unpriced": 0}
		}
		session := func(id, title string, scale float64) map[string]any {
			s := map[string]any{"id": id, "title": title, "project": "p1", "provider": "openai", "model": "gpt-5.5", "createdAt": day(3).Format(time.RFC3339),
				"updatedAt": day(6).Add(10 * time.Hour).Format(time.RFC3339), "archived": false,
				"context": []any{map[string]any{"time": day(6).Add(10 * time.Hour).Format(time.RFC3339), "input": 30000, "cache_read": 20000, "output": 900}}}
			for k, v := range counts(scale) {
				s[k] = v
			}
			return s
		}
		var dailies []any
		for i := range 7 {
			c := counts(float64(i%3 + 1))
			c["time"] = day(i).Format(time.DateOnly)
			dailies = append(dailies, c)
		}
		return map[string]any{"rows": []any{session("registered", "Usage port review", 2), session("s2", "Heatmap layout", 1)}, "sum": counts(3),
			"series": dailies, "by": map[string]any{"model": map[string]any{"gpt-5.5": counts(3)}, "provider": map[string]any{"openai": counts(3)},
				"project": map[string]any{"p1": counts(2), "": counts(1)}, "hour": map[string]any{"10": counts(2), "21": counts(1)}},
			"total": 2, "offset": 0, "limit": 15, "median": 98000, "p90": 196000, "top": []any{session("registered", "Usage port review", 2)},
			"tools": map[string]any{"read": 12, "bash": 7}, "skills": map[string]any{"code-rules": 2}, "warnings": []any{},
			"shape": map[string]any{"messages": map[string]any{"edges": []int{1, 6, 16, 31, 61, 121}, "counts": []int{1, 3, 6, 2, 1, 0}, "total": 13}}}, nil
	}
	f.u.revision++
	f.settle()
	return f
}

// views are the tab's views by their labels.
var views = []string{"概览", "请求明细", "上下文", "会话", "模型归并"}

func viewLabels() []string {
	if i18n.Locale() == i18n.English {
		return []string{"Overview", "Requests", "Context", "Sessions", "Model aliases"}
	}
	return views
}

// TestNarrowWindowsKeepEveryTextInside lays every view out at narrow
// widths, in both languages, and checks that nothing reaches past the
// window's right edge: no view needs scrolling sideways.
func TestNarrowWindowsKeepEveryTextInside(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, width := range []int{640, 480} {
			f := richFixture(t, width, 9000)
			i18n.SetLocale(locale)
			for i, label := range viewLabels() {
				f.u.view = i
				f.settle()
				if i == viewRequests {
					f.click(dateTime(f.u.ledger.page.Rows[1].Time))
				}
				if i == viewContext {
					f.click("Usage port review")
				}
				for _, text := range f.tt.Texts() {
					box, ok := f.tt.Find(text)
					if ok && (box.X < -0.5 || box.X+box.W > float32(width)+0.5) {
						t.Errorf("%s %d %s: %q is at %v..%v", locale, width, label, text, box.X, box.X+box.W)
					}
				}
			}
		}
	}
}

// TestScreenshots writes PNGs of the views to the directory named by
// WINONE_USAGE_SHOTS, for looking at: light and dark, Chinese and
// English, wide and narrow. It is skipped otherwise.
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("WINONE_USAGE_SHOTS")
	if dir == "" {
		t.Skip("WINONE_USAGE_SHOTS is not set")
	}
	defer i18n.SetLocale(i18n.Chinese)
	for _, c := range []struct {
		locale string
		dark   bool
		width  int
	}{{i18n.Chinese, false, 1100}, {i18n.Chinese, true, 1100}, {i18n.English, false, 1100}, {i18n.Chinese, false, 700}, {i18n.English, true, 520}} {
		f := richFixture(t, c.width, 6000)
		i18n.SetLocale(c.locale)
		// MyGo's own words (a search field's placeholder) follow the
		// window's language, as the app's window would set it.
		f.tt.SetLocale(c.locale)
		f.dark = c.dark
		for i := range viewLabels() {
			f.u.view = i
			f.settle()
			switch i {
			case viewRequests:
				f.click(dateTime(f.u.ledger.page.Rows[1].Time))
				f.h.Picked["导出请求归档"] = ""
			case viewContext:
				f.click("Usage port review")
			case viewAliases:
				f.opts.Preferences.Aliases = map[string]string{"deepseek-v4.1-flash": "deepseek-flash", "gpt-5.5-2026-10": "gpt-5.5"}
				f.u.aliases.from = "kimi-k2.5-preview"
				f.settle()
			}
			theme := "light"
			if c.dark {
				theme = "dark"
			}
			name := fmt.Sprintf("%s-%s-%d-%d-%s", strings.ToLower(c.locale[:2]), theme, c.width, i, []string{"overview", "requests", "context", "sessions", "aliases"}[i])
			shot(t, f, filepath.Join(dir, name+".png"))
			// The pointer on a day of the trend, and of the heatmap.
			series := f.u.ledger.page.Series
			if (i == viewOverview || i == viewRequests) && len(series) > 3 {
				target := ledWhen(series[3], "day")
				if i == viewRequests {
					day, _ := localDay(f.u.ledger.page.Heatmap.To)
					target = formatDate(day.AddDate(0, 0, -14), "yMMMd")
				}
				if box, ok := f.tt.Find(target); ok {
					f.tt.Move(box.X+box.W/2, box.Y+box.H/2)
					f.settle()
					shot(t, f, filepath.Join(dir, name+"-hover.png"))
					f.tt.Move(1, 1)
					f.settle()
				}
			}
		}
	}
}

// shot writes the window's last frame, cut below its content.
func shot(t *testing.T, f *fixture, path string) {
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, cropToContent(f.tt.Image())); err != nil {
		t.Fatal(err)
	}
}

// cropToContent cuts the empty window below the content, which is all
// the background color of its last row.
func cropToContent(img *image.RGBA) image.Image {
	b := img.Bounds()
	background := img.RGBAAt(b.Min.X, b.Max.Y-1)
	last := b.Min.Y
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) != background {
				last = y
				break
			}
		}
		if last != b.Min.Y {
			break
		}
	}
	return img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, min(b.Max.Y, last+32)))
}

package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/ai"
)

func TestUsageDateRangeIncludesBothWholeDaysAndExcludesLaterMessages(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", t.TempDir(), "Date range", ai.Selection{Provider: "fixture", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.SessionDirectory, "session.jsonl")
	var rows []byte
	for _, at := range []time.Time{time.Date(2026, 10, 7, 23, 59, 59, 0, time.Local), time.Date(2026, 10, 8, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 9, 23, 59, 59, 0, time.Local), time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local)} {
		entry, _ := json.Marshal(map[string]any{"type": "message", "timestamp": at.Format(time.RFC3339Nano), "message": map[string]any{"role": "assistant", "usage": map[string]int{"input": 10, "output": 5}, "content": []any{}}})
		rows = append(rows, append(entry, '\n')...)
	}
	if err = os.WriteFile(path, rows, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindSession(c.ID, path); err != nil {
		t.Fatal(err)
	}
	page, err := store.Usage(UsageQuery{Period: "2026-10-08..2026-10-09"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Sum.Calls != 2 || page.Sum.Input != 20 || page.Sum.Output != 10 || len(page.Series) != 2 {
		t.Fatalf("range boundaries changed: %+v", page)
	}
	for _, period := range []string{"2026-10-09..2026-10-08", "2026-02-30..2026-03-01", "2026-10-08.."} {
		if _, err := store.Usage(UsageQuery{Period: period}); err == nil {
			t.Fatalf("invalid range accepted: %s", period)
		}
	}
}

func TestUsageGroupsResolvedAliasesButKeepsOriginalSessionModels(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]UsagePrice{}
	for i, model := range []string{"vendor/old-model", "new-model"} {
		c, err := store.NewConversation("", t.TempDir(), model, ai.Selection{Provider: "fixture", Model: model})
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := json.Marshal(map[string]any{"type": "message", "timestamp": time.Now().Format(time.RFC3339Nano), "message": map[string]any{"role": "assistant", "usage": map[string]int{"input": 10, "output": 5}, "content": []any{}}})
		path := filepath.Join(c.SessionDirectory, "session.jsonl")
		if err := os.WriteFile(path, append(entry, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.BindSession(c.ID, path); err != nil {
			t.Fatal(err)
		}
		prices[c.ID] = UsagePrice{Cost: float64(i + 1), PricedCalls: 1}
	}
	aliases := map[string]string{"vendor/old-model": "new-model", "new-model": "new-model"}
	for _, filter := range []string{"", "new-model", "vendor/old-model"} {
		page, err := store.Usage(UsageQuery{Period: "all", Model: filter, ModelAliases: aliases}, prices)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 2 || len(page.By["model"]) != 1 || page.By["model"]["new-model"].Calls != 2 || page.Sum.Cost != 3 {
			t.Fatalf("alias report: %+v", page)
		}
		originals := map[string]bool{}
		for _, row := range page.Rows {
			originals[row.Model] = true
		}
		if !originals["vendor/old-model"] || !originals["new-model"] {
			t.Fatal("session model was renamed")
		}
	}
	var external UsageQuery
	if err := json.Unmarshal([]byte(`{"modelAliases":{"vendor/old-model":"spoofed"}}`), &external); err != nil {
		t.Fatal(err)
	}
	if len(external.ModelAliases) != 0 {
		t.Fatal("accepted client-supplied alias resolution")
	}
}

func TestUsageReadsOnlyRegisteredSessionsAndKeepsPageTotals(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 16; n++ {
		c, err := store.NewConversation("", t.TempDir(), "Registered", ai.Selection{Provider: "fixture", Model: "model"})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(c.SessionDirectory, "session.jsonl")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		entry := map[string]any{"type": "message", "timestamp": now, "message": map[string]any{"role": "assistant", "usage": map[string]int{"input": 10, "output": 5, "cacheRead": 3, "cacheWrite": 2, "reasoning": 1}, "content": []any{map[string]any{"type": "toolCall", "name": "read", "arguments": map[string]string{"path": "X:/skills/example/SKILL.md"}}}}}
		raw, _ := json.Marshal(entry)
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.BindSession(c.ID, path); err != nil {
			t.Fatal(err)
		}
	}
	// An unrelated session-like file beside the catalog is never enumerated.
	if err := os.WriteFile(filepath.Join(root, "foreign.jsonl"), []byte("unreadable foreign data"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := store.Usage(UsageQuery{Period: "7d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) != 15 || first.Total != 16 || first.Sum.Input != 160 || first.Sum.Output != 80 || first.Tools["read"] != 16 || first.Skills["example"] != 16 {
		t.Fatalf("unexpected report: %+v", first)
	}
	second, err := store.Usage(UsageQuery{Period: "7d", Offset: 15})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Rows) != 1 || second.Sum != first.Sum {
		t.Fatal("page changed totals")
	}
	filtered, err := store.Usage(UsageQuery{Period: "7d", Model: "other"})
	if err != nil || filtered.Total != 0 {
		t.Fatal("model filter ignored")
	}
}

func TestUsageDoesNotPresentCorruptionAsEmptySuccess(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", t.TempDir(), "Damaged", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.SessionDirectory, "session.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BindSession(c.ID, path); err != nil {
		t.Fatal(err)
	}
	report, err := store.Usage(UsageQuery{Period: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 || report.Rows[0].Error == "" {
		t.Fatal("read error hidden")
	}
	if err := os.WriteFile(path, []byte(`{"type":"message"`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err = store.Usage(UsageQuery{Period: "all"})
	if err != nil || !report.Rows[0].Partial {
		t.Fatal("partial tail not reported")
	}
}

func TestUsageEmptyCatalogEncodesArraysInsteadOfNull(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Usage(UsageQuery{Period: "7d"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	// The settings UI iterates these fields directly; null crashed the page.
	for _, name := range []string{"rows", "top", "series", "warnings"} {
		if got := string(fields[name]); got != "[]" {
			t.Fatalf("%s must encode as [] without any session, got %s", name, got)
		}
	}
	for _, name := range []string{"by", "tools", "skills"} {
		if got := string(fields[name]); got != "{}" {
			t.Fatalf("%s must encode as {} without any session, got %s", name, got)
		}
	}
}

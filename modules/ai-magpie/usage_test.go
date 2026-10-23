package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type usagePriceTransport func(*http.Request) (*http.Response, error)

func TestGatewayCallerIdentityReachesLedgerButNeverVendor(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(func() {
		catalog.Reset()
		usage.WinOnePath = ""
		usage.WinOneRecord = nil
		provider.WinOneProviders = nil
	})
	requests := 0
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-WinOne-Caller-Key") != "" {
			t.Error("private accounting header reached vendor")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"fixture","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5}}`)
	}))
	defer vendor.Close()
	id := strings.Repeat("a", 64)
	for _, key := range []string{id, "invalid-identity", ""} {
		in := request{Operation: "gateway", ProviderID: "fixture", Protocol: "chat", BaseURL: vendor.URL + "/v1", Key: "fixture-vendor-key", Model: "gpt-4o", GatewayPath: "/v1/chat/completions", GatewayHeaders: make(http.Header), GatewayBody: json.RawMessage(`{"messages":[{"role":"user","content":"fixture"}],"stream":false}`)}
		in.GatewayHeaders.Set("X-WinOne-Caller-Key", key)
		var out bytes.Buffer
		if err := serveGateway(context.Background(), in, &out); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 3 {
		t.Fatalf("vendor request count=%d", requests)
	}
	for key, count := range map[string]int{id: 1, "none": 2} {
		var out bytes.Buffer
		if err := readUsage(request{Usage: &usageQuery{Period: "all", CallerKey: key}}, &out); err != nil {
			t.Fatal(err)
		}
		var page struct {
			Total int
			Rows  []usage.Row
		}
		if err := json.Unmarshal(out.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Total != count {
			t.Fatalf("caller %s count=%d want %d: %s", key, page.Total, count, out.String())
		}
	}
}

func TestUsagePreservesReportedPlanCreditsAndServiceTimings(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil }()
	if err := configureUsage(request{ProviderID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	for _, credit := range []float64{0, 0.25} {
		got := usage.WinOneRecord(usage.Record{Credits: &credit, TTFT: 100, FirstText: 300, Flow: 500, TierAsked: "priority", TierSent: "ultrafast", TierServed: "ultrafast", Error: "private"})
		if got.Credits == nil || *got.Credits != credit || got.FirstText != 300 || got.Flow != 500 || got.TierServed != "ultrafast" || got.Error != "Request failed" {
			t.Fatalf("usage metadata dropped: %+v", got)
		}
	}
}

func TestUsageScopesGatewayKeysAndCallsWithoutKeys(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil }()
	if err := configureUsage(request{ProviderID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two", ""} {
		usage.Append(usage.Record{Time: time.Now(), Model: "fixture", CallerKeyID: id, CallerKeyName: id, Status: 200})
	}
	for _, id := range []string{"one", "two", usage.NoCallerKey} {
		q := usageQuery{Period: "all"}
		raw, _ := json.Marshal(map[string]string{"period": "all", "callerKey": id})
		if err := json.Unmarshal(raw, &q); err != nil {
			t.Fatal(err)
		}
		page := usage.QueryPage(usage.Period("all"), q.filter(), 0, 15)
		if page.Total != 1 {
			t.Fatalf("key filter %q returned %d calls", id, page.Total)
		}
	}
}

func (f usagePriceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const usagePriceFixture = `{"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o","cost":{"input":2,"output":8}}}}}`

func TestUsageRangeAndHeatmapUseTheSameFilteredLedger(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(func() { catalog.Reset(); usage.WinOnePath = ""; usage.WinOneRecord = nil })
	if err := configureUsage(request{ProviderID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, -3)
	for _, offset := range []int{-1, 0, 1} {
		usage.Append(usage.Record{Time: day.AddDate(0, 0, offset), Model: "gpt-4o", Input: 20, Output: 5, Status: 200})
	}
	date := day.Format(time.DateOnly)
	var output bytes.Buffer
	if err := readUsage(request{Usage: &usageQuery{Period: date + ".." + date, Provider: "fixture"}}, &output); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Total   int
		Heatmap usage.Heatmap
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("custom range includes calls outside its whole day: %d", page.Total)
	}
	if len(page.Heatmap.Days) != 3 {
		t.Fatalf("53-week heatmap lost requests outside the selected day: %+v", page.Heatmap)
	}
	for _, day := range page.Heatmap.Days {
		if day.Calls != 1 || day.Tokens != 25 {
			t.Fatalf("heatmap totals changed: %+v", day)
		}
	}
	output.Reset()
	if err := readUsage(request{Usage: &usageQuery{Period: "7d", Provider: "another"}}, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Heatmap.Days) != 0 {
		t.Fatal("heatmap ignored the provider filter")
	}
}

func TestUsageFullExportAndContextStayScoped(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	defer catalog.Reset()
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil }()
	in := request{Operation: "gateway", UsageSource: "winone", ProviderID: "fixture", Model: "gpt-4o", GatewayPath: "/v1/chat/completions", GatewayHeaders: http.Header{"X-Winone-Session": []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ"}}, GatewayBody: json.RawMessage(`{"messages":[{"role":"system","content":"private instruction"},{"role":"user","content":"private prompt"}],"tools":[{"type":"function","function":{"name":"read","parameters":{"type":"object"}}}]}`), ModelMetadata: modelMetadata{ContextWindow: 1000}}
	if err := configureUsage(in); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 520; n++ {
		usage.Append(usage.Record{Time: time.Now(), Model: "gpt-4o", Input: 20, Output: 5, Status: 200})
	}
	var output bytes.Buffer
	if err := readUsage(request{Usage: &usageQuery{Period: "7d", Provider: "fixture", Export: true}}, &output); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Rows    []usage.Row
		Limit   int
		Total   int
		CSV     string
		Context ctxJSON
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 15 || page.Limit != 15 || page.Total != 520 {
		t.Fatal("pagination not 15")
	}
	rows, err := csv.NewReader(strings.NewReader(page.CSV)).ReadAll()
	if err != nil || len(rows) != 521 {
		t.Fatalf("export truncated at page limit: %d %v", len(rows), err)
	}
	output.Reset()
	if err := readUsage(request{Usage: &usageQuery{Period: "7d", Provider: "fixture"}}, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Context.Sessions) != 1 || page.Context.Sessions[0].Requests != 520 || page.Context.Sessions[0].Latest.Window != 1000 {
		t.Fatal("context did not preserve registered session and window")
	}
	raw, err := os.ReadFile(usage.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") {
		t.Fatal("prompt body leaked into ledger")
	}
}

func TestUsageViewerZoneKeepsDayFilterHeatmapAndSessionCostTogether(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(func() { catalog.Reset(); usage.WinOnePath = ""; usage.WinOneRecord = nil })
	if err := configureUsage(request{ProviderID: "fixture", UsageSource: "winone", GatewayHeaders: http.Header{"X-Winone-Session": []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ"}}}); err != nil {
		t.Fatal(err)
	}
	// The same call belongs to different calendar days on opposite sides of
	// midnight. Keep it recent so the rolling heatmap includes the fixture.
	day := time.Now().UTC().AddDate(0, 0, -3)
	stamp := time.Date(day.Year(), day.Month(), day.Day(), 23, 30, 0, 0, time.UTC)
	usage.Append(usage.Record{Time: stamp, Model: "gpt-4o", Input: 20, Output: 5, Status: 200})
	for _, zone := range []string{"Pacific/Honolulu", "Asia/Tokyo"} {
		date := stamp.In(usage.Zone(zone)).Format(time.DateOnly)
		var output bytes.Buffer
		if err := readUsage(request{Usage: &usageQuery{Period: date + ".." + date, Zone: zone, Day: date}}, &output); err != nil {
			t.Fatal(err)
		}
		var page struct {
			Total        int
			Heatmap      usage.Heatmap
			SessionCosts map[string]struct{ Days map[string]float64 }
		}
		if err := json.Unmarshal(output.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Heatmap.Days) != 1 || len(page.SessionCosts) != 1 {
			t.Fatalf("%s lost the call at the viewer's day boundary: %s", zone, output.String())
		}
		if page.Heatmap.Days[0].Date != date {
			t.Fatalf("%s heatmap used another day: %+v", zone, page.Heatmap)
		}
		for _, price := range page.SessionCosts {
			if len(price.Days) != 1 || price.Days[date] <= 0 {
				t.Fatalf("%s session cost used another day: %+v", zone, price)
			}
		}
	}
}

func TestChatProtocolCallsKeepWinOneUsageOrigin(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil }()
	for _, tc := range []struct{ source, want string }{{"winone", "winone"}, {"", "gateway"}, {"gateway", "gateway"}} {
		if err := configureUsage(request{Operation: "gateway", UsageSource: tc.source, ProviderID: "test"}); err != nil {
			t.Fatal(err)
		}
		record := usage.WinOneRecord(usage.Record{Agent: "upstream-account", Provider: "private-account", Model: "model", Input: 20, Output: 5, Error: "private upstream text"})
		if record.Agent != tc.want || record.Provider != "test" || record.Input != 20 || record.Output != 5 || record.Error != "Request failed" {
			t.Fatalf("invalid usage attribution: %#v", record)
		}
	}
}

func TestUsageRefreshesPricesWithoutLosingUsageOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cached   bool
		status   int
		wantCost float64
		warning  bool
	}{
		{"cold catalog", false, http.StatusOK, 0.00008, false},
		{"offline with cache", true, http.StatusServiceUnavailable, 0.00008, true},
		{"offline without cache", false, http.StatusServiceUnavailable, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
			t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
			t.Setenv("USERPROFILE", filepath.Join(root, "home"))
			t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(root, "usage.jsonl"))
			priorProviders, priorClient := provider.WinOneProviders, http.DefaultClient
			provider.WinOneProviders = []provider.Provider{}
			catalog.Reset()
			t.Cleanup(func() {
				provider.WinOneProviders, http.DefaultClient = priorProviders, priorClient
				usage.WinOnePath, usage.WinOneRecord = "", nil
				catalog.Reset()
			})
			if tc.cached {
				if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0o600); err != nil {
					t.Fatal(err)
				}
				old := time.Now().Add(-48 * time.Hour)
				if err := os.Chtimes(catalog.CachePath(), old, old); err != nil {
					t.Fatal(err)
				}
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, usagePriceFixture)
			}))
			defer server.Close()
			http.DefaultClient = &http.Client{Transport: usagePriceTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://models.dev/api.json" {
					return nil, fmt.Errorf("unexpected price request: %s", r.URL)
				}
				local := r.Clone(r.Context())
				local.URL.Scheme, local.URL.Host = "http", strings.TrimPrefix(server.URL, "http://")
				return server.Client().Transport.RoundTrip(local)
			})}
			if err := configureUsage(request{}); err != nil {
				t.Fatal(err)
			}
			usage.Append(usage.Record{Time: time.Now(), Provider: "fixture", Model: "gpt-4o", Input: 20, Output: 5, Status: 200})
			var out bytes.Buffer
			if err := readUsage(request{Usage: &usageQuery{Period: "all"}}, &out); err != nil {
				t.Fatal(err)
			}
			var page struct {
				Sum          usage.Totals
				PriceWarning bool
			}
			if err := json.Unmarshal(out.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if requests != 1 || page.Sum.Calls != 1 || page.Sum.Input != 20 || page.Sum.Output != 5 || page.Sum.Cost != tc.wantCost || page.PriceWarning != tc.warning {
				t.Fatalf("price refresh requests=%d, result=%s", requests, out.String())
			}
			if tc.status == http.StatusOK {
				if err := readUsage(request{Usage: &usageQuery{Period: "all"}}, io.Discard); err != nil {
					t.Fatal(err)
				}
				if requests != 1 {
					t.Fatal("a fresh price cache was downloaded again")
				}
			}
		})
	}
}

func TestUsageCollectsRealGatewayCallsAcrossProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	t.Setenv("WINONE_AI_USAGE_PATH", path)
	// These accounting assertions use a fresh local tariff; no online request.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(usagePriceFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil; provider.WinOneProviders = nil }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{\"id\":\"reply\",\"model\":\"gpt-4o\",\"choices\":[{\"message\":{\"content\":\"Hello\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":5}}")
	}))
	defer server.Close()
	for _, id := range []string{"first", "second"} {
		in := request{Protocol: "chat", BaseURL: server.URL + "/v1", Key: "private-key", Model: "gpt-4o", Text: "private prompt", ProviderID: id}
		raw, _ := json.Marshal(in)
		if _, err := run(context.Background(), bytes.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") {
		t.Fatal("private input leaked into usage ledger")
	}
	var output bytes.Buffer
	if err := readUsage(request{Usage: &usageQuery{Period: "all"}}, &output); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Total int
		Sum   usage.Totals
		Rows  []usage.Row
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Sum.Calls != 2 || page.Sum.Input != 40 || page.Sum.Output != 10 || len(page.Rows) != 2 {
		t.Fatalf("wrong accounting: %s", output.String())
	}
	output.Reset()
	if err := readUsage(request{Usage: &usageQuery{Period: "all", Provider: "second"}}, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Rows[0].Provider != "second" {
		t.Fatalf("wrong provider filter: %s", output.String())
	}
	output.Reset()
	if err := readUsage(request{Usage: &usageQuery{Period: "all", Offset: 1}}, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Rows) != 1 || page.Sum.Calls != 2 {
		t.Fatal("pagination changed totals")
	}
}

func TestUsageReportsEmptyAndRejectsInvalidFilters(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "empty.jsonl"))
	defer func() { usage.WinOnePath = ""; usage.WinOneRecord = nil }()
	for _, query := range []usageQuery{{Period: "invalid"}, {Period: "all", Offset: -1}, {Period: "all", Source: "invalid"}} {
		if err := readUsage(request{Usage: &query}, io.Discard); err == nil {
			t.Fatal("invalid filter accepted", query)
		}
	}
	var out bytes.Buffer
	if err := readUsage(request{Usage: &usageQuery{Period: "all"}}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"rows\":[]") {
		t.Fatal(out.String())
	}
}

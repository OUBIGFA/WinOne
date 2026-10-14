package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/usage"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Magpie remains responsible for token accounting, prices, aggregation and
// pagination. Only installation-owned metadata enters the shared WinOne log.
func configureUsage(in request) error {
	path := os.Getenv("WINONE_AI_USAGE_PATH")
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return errors.New("invalid usage path")
	}
	usage.WinOnePath = path
	source := "winone"
	if in.Operation == "gateway" && in.UsageSource != "winone" {
		source = "gateway"
	}
	usage.WinOneRecord = func(r usage.Record) usage.Record {
		failure := ""
		if r.Error != "" {
			failure = "Request failed"
		}
		provider := in.ProviderID
		if provider == "" {
			provider = "legacy"
		}
		body, path := in.GatewayBody, in.GatewayPath
		if len(body) == 0 {
			messages := []map[string]string{{"role": "system", "content": in.System}}
			if len(in.Messages) == 0 {
				messages = append(messages, map[string]string{"role": "user", "content": in.Text})
			} else {
				for _, m := range in.Messages {
					messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
				}
			}
			body, _ = json.Marshal(map[string]any{"messages": messages})
			path = "/v1/chat/completions"
		}
		window := in.ModelMetadata.ContextWindow
		if window == 0 {
			window = in.ModelMetadata.ListedContextWindow
		}
		var prompt json.RawMessage
		if p := gateway.WinOnePrompt(path, body, r.Input+r.CacheRead+r.CacheWrite, window); p != nil {
			prompt, _ = json.Marshal(p)
		}
		session := ""
		if in.UsageSource == "winone" {
			candidate := in.GatewayHeaders.Get("X-WinOne-Session")
			if len(candidate) == 26 && !strings.ContainsAny(candidate, "/\\\x00") {
				session = candidate
			}
		}
		if session == "" {
			session = gateway.WinOneConversationID(body)
		}
		accountID := ""
		if in.Agent != "" {
			accountID = provider
		}
		// Keep routing/timing metadata, never prompt bodies, credentials or raw
		// upstream errors. WinOne's connection ID is the account filter identity.
		return usage.Record{Time: r.Time, Agent: source, Provider: provider, Model: r.Model,
			WinOnePrompt: prompt, Session: session,
			Requested: r.Requested, Served: r.Served, Effort: r.Effort, Endpoint: r.Endpoint, Stop: r.Stop,
			ProviderAccount: accountID, Kind: r.Kind,
			Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite: r.CacheWrite,
			CacheWrite1h: r.CacheWrite1h, Reasoning: r.Reasoning, Millis: r.Millis,
			TTFT: r.TTFT, FirstText: r.FirstText, Sent: r.Sent, Flow: r.Flow, Status: r.Status, Error: failure, Rejected: r.Rejected}
	}
	return nil
}

type usageQuery struct {
	Period   string `json:"period"`
	Provider string `json:"provider"`
	Source   string `json:"source"`
	Failed   bool   `json:"failed"`
	Offset   int    `json:"offset"`
	Day      string `json:"day"`
	Model    string `json:"model"`
	Account  string `json:"account"`
	Purpose  string `json:"purpose"`
	Query    string `json:"query"`
	Export   bool   `json:"export,omitempty"`
}

func (q usageQuery) filter() usage.Filter {
	// Through is fixed: upstream session discovery must never scan another
	// client's profile. WinOne Chat has its own registered-session reader.
	return usage.Filter{Provider: q.Provider, Agent: q.Source, Failed: q.Failed, Through: usage.SourceGateway, Day: q.Day, Model: q.Model, Account: q.Account, Purpose: q.Purpose, Query: q.Query}
}

func readUsage(in request, output io.Writer) error {
	if err := configureUsage(in); err != nil {
		return err
	}
	// A source filter alone is too late: upstream enumerates session sources
	// before applying it. Use its injected reader to skip that discovery.
	priorLogCalls, priorProviders := usage.LogCalls, provider.WinOneProviders
	usage.LogCalls = func(time.Time) []sessions.Call { return nil }
	provider.WinOneProviders = []provider.Provider{}
	defer func() { usage.LogCalls = priorLogCalls; provider.WinOneProviders = priorProviders }()
	q := in.Usage
	if q == nil {
		return errors.New("usage query required")
	}
	if !usage.Period(q.Period).Known() {
		return errors.New("invalid usage period")
	}
	if q.Offset < 0 || q.Offset > 1000000 || len(q.Provider) > 256 || (q.Source != "" && q.Source != "winone" && q.Source != "gateway") {
		return errors.New("invalid usage filter")
	}
	if len(q.Model) > 512 || len(q.Account) > 256 || len(q.Purpose) > 128 || len(q.Query) > 512 || strings.ContainsRune(q.Query, '\x00') {
		return errors.New("invalid usage filter")
	}
	if q.Day != "" {
		if _, err := time.Parse("2006-01-02", q.Day); err != nil {
			return errors.New("invalid usage day")
		}
	}
	// A missing ledger is the initial empty state; an inaccessible ledger is an
	// error, not a successful zero-usage result.
	f, err := os.Open(usage.Path())
	if err != nil && !os.IsNotExist(err) {
		return errors.New("usage ledger could not be read")
	}
	if f != nil {
		if err = f.Close(); err != nil {
			return err
		}
	}
	page := usage.QueryPage(usage.Period(q.Period), q.filter(), q.Offset, 15)
	priceWarning := false
	if page.Sum.Calls > 0 && catalog.Stale() {
		// The upstream GUI keeps this cache fresh in its long-running backend.
		// Our short-lived reader must finish the same refresh before pricing.
		// A failed public-catalog request must not hide the local usage ledger.
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := catalog.Sync(ctx)
		cancel()
		priceWarning = err != nil
		if err == nil {
			page = usage.QueryPage(usage.Period(q.Period), q.filter(), q.Offset, 15)
		}
	}
	if page.Rows == nil {
		page.Rows = []usage.Row{}
	}
	if q.Export {
		// CSV is consumed inside the Host, not returned through a UI frame.
		// Avoid attaching unrelated context graphs to a potentially large export.
		if page.Total > 100000 {
			return errors.New("too many requests to export; narrow the period")
		}
		all := usage.LedgerOf(usage.Period(q.Period), q.filter())
		var csv bytes.Buffer
		if err := usage.WriteCSV(&csv, all.Rows); err != nil {
			return err
		}
		if csv.Len() > 64<<20 {
			return errors.New("CSV export is too large; narrow the period")
		}
		return json.NewEncoder(output).Encode(map[string]any{"rows": page.Rows, "sum": page.Sum, "total": page.Total, "offset": q.Offset, "limit": 15, "csv": csv.String()})
	}
	if page.By == nil {
		page.By = map[string][]usage.Share{}
	}
	page.By["account"] = []usage.Share{}
	type rowView struct {
		usage.Row
		Speed float64 `json:"speed"`
	}
	rows := make([]rowView, 0, len(page.Rows))
	for _, r := range page.Rows {
		n, ms := r.Decode()
		speed := 0.0
		if ms > 0 {
			speed = float64(n) * 1000 / float64(ms)
		}
		rows = append(rows, rowView{Row: r, Speed: speed})
	}
	result := map[string]any{"rows": page.Rows, "sum": page.Sum, "total": page.Total, "offset": q.Offset, "limit": 15, "by": page.By, "chartBy": page.ChartBy, "series": page.Series, "bucket": page.Bucket, "day": q.Day, "providers": page.Providers, "agents": page.Agents, "accounts": page.Accounts, "purposes": page.Purposes, "callerKeys": page.CallerKeys, "through": page.Through, "priceWarning": priceWarning}
	contexts := []gateway.Route{}
	// HeatmapOf reuses the upstream request index and all the same filters,
	// while deliberately showing its complete 53-week span beyond this period.
	result["heatmap"] = usage.HeatmapOf(q.filter())
	result["rows"] = rows
	type sessionPrice struct {
		Cost        float64            `json:"cost"`
		PricedCalls int                `json:"pricedCalls"`
		Days        map[string]float64 `json:"days"`
	}
	prices := map[string]*sessionPrice{}
	ledger := usage.LedgerOf(usage.Period(q.Period), q.filter())
	accountRows := map[string][]usage.Record{}
	for _, r := range ledger.Rows {
		if r.Account() != "" && !r.IsRejected() {
			accountRows[r.Account()] = append(accountRows[r.Account()], r.Record)
		}
	}
	pricer := usage.NewPricer()
	for id, records := range accountRows {
		page.By["account"] = append(page.By["account"], usage.Share{ID: id, Totals: pricer(records)})
	}
	slices.SortFunc(page.By["account"], func(a, b usage.Share) int { return b.Totals.AllTokens() - a.Totals.AllTokens() })
	for _, r := range ledger.Rows {
		if r.Session == "" || !r.Priced || r.IsRejected() {
			continue
		}
		p := prices[r.Session]
		if p == nil {
			p = &sessionPrice{Days: map[string]float64{}}
			prices[r.Session] = p
		}
		p.Cost += r.Cost
		p.PricedCalls++
		p.Days[r.Time.Local().Format("2006-01-02")] += r.Cost
	}
	result["sessionCosts"] = prices
	for _, r := range usage.Load(usage.Period(q.Period).Since(time.Now())) {
		if len(r.WinOnePrompt) == 0 || q.Provider != "" && q.Provider != r.Provider || q.Source != "" && q.Source != r.Agent || q.Model != "" && q.Model != r.Model {
			continue
		}
		var prompt gateway.Prompt
		if json.Unmarshal(r.WinOnePrompt, &prompt) != nil {
			continue
		}
		contexts = append(contexts, gateway.Route{ID: int64(len(contexts) + 1), Time: r.Time, Agent: r.Provider, Provider: r.Provider, Model: r.Model, Session: r.Session, Prompt: &prompt, Done: true, Status: r.Status, Error: r.Error, Usage: []gateway.RouteUsage{{CacheRead: r.CacheRead, Input: r.Input, Output: r.Output}}})
	}
	contextPage := contextOf(contexts, 0)
	contextTotal := len(contextPage.Sessions)
	contextStart := min(q.Offset, contextTotal)
	contextPage.Sessions = contextPage.Sessions[contextStart:min(contextStart+15, contextTotal)]
	result["context"] = contextPage
	result["contextTotal"] = contextTotal
	// Detail records contain Magpie's classified item names/counts only.
	visibleRequests := map[int64]bool{}
	for _, session := range contextPage.Sessions {
		for _, point := range session.Points {
			visibleRequests[point.ID] = true
		}
	}
	details := []gateway.Route{}
	for _, route := range contexts {
		if visibleRequests[route.ID] {
			details = append(details, route)
		}
	}
	result["contextRequests"] = details
	return json.NewEncoder(output).Encode(result)
}

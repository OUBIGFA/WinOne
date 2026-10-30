package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/gui"
	"github.com/yetone/magpie/internal/maskrec"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/usage"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type usageAliasEdit struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Only archive metadata supplied by our ledger is accepted. Upstream owns
// name validation, local-file access and the bounded body preview. Reading
// an existing file does not load plugins or enable collection.
func readArchive(in request, output io.Writer) error {
	if err := configureUsage(in); err != nil {
		return err
	}
	date, id, _ := strings.Cut(in.Archive, "/")
	name, ok := gateway.ArchiveName(date, id)
	if !ok {
		return errors.New("invalid archive id")
	}
	if in.Operation == "archive.file" {
		body, _, err := gateway.LocalArchive().Open(context.Background(), name)
		if err != nil {
			return err
		}
		defer body.Close()
		_, err = io.Copy(output, body)
		return err
	}
	target := "http://127.0.0.1/api/archive?" + url.Values{"date": {date}, "id": {id}}.Encode()
	response := httptest.NewRecorder()
	gui.WinOneArchiveHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		return errors.New("archive preview unavailable")
	}
	_, err := io.Copy(output, response.Body)
	return err
}

// Validation is the upstream CLI's SetUsageAlias, including canonical names,
// alias chains, self-reference rejection and removal. Its settings writer is
// redirected only for this in-memory operation; no account or plugin is loaded.
func editUsageAliases(in request, out io.Writer) error {
	if len(in.UsageAliases) > 4096 {
		return errors.New("too many usage aliases")
	}
	settings.WinOneUse(settings.Settings{NoStats: true, NoUsageStats: true})
	keys := slices.Sorted(maps.Keys(in.UsageAliases))
	slices.Reverse(keys)
	// aliasIndex reads the first sorted spelling of a duplicate canonical name.
	// Apply it last through the same CLI setter, then apply the user's edit.
	for _, from := range keys {
		if _, err := provider.SetUsageAlias(from, in.UsageAliases[from]); err != nil {
			return err
		}
	}
	if in.UsageAlias != nil {
		if _, err := provider.SetUsageAlias(in.UsageAlias.From, in.UsageAlias.To); err != nil {
			return err
		}
	}
	aliases := provider.UsageAliases()
	if aliases == nil {
		aliases = map[string]string{}
	}
	return json.NewEncoder(out).Encode(map[string]any{"usageAliases": aliases})
}

// Magpie remains responsible for token accounting, prices, aggregation and
// pagination. Only installation-owned metadata enters the shared WinOne log.
func configureUsage(in request) error {
	gateway.WinOneArchiveDirectory = ""
	path := os.Getenv("WINONE_AI_USAGE_PATH")
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return errors.New("invalid usage path")
	}
	usage.WinOnePath = path
	gateway.WinOneArchiveDirectory = filepath.Join(filepath.Dir(path), "request-archive")
	// Every owned worker shares metadata records with Privacy, like usage.
	// Upstream never writes original values or surrounding text to these files.
	maskrec.Dir = func() string { return filepath.Join(filepath.Dir(path), "masking") }
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
		return usage.Record{Time: r.Time, Agent: source, Provider: provider, Model: r.Model, Archive: r.Archive,
			WinOnePrompt: prompt, Session: session,
			Requested: r.Requested, Served: r.Served, Effort: r.Effort, Endpoint: r.Endpoint, Stop: r.Stop,
			ProviderAccount: accountID, Kind: r.Kind,
			CallerKeyID: r.CallerKeyID, CallerKeyName: r.CallerKeyName,
			Credits: r.Credits, TierAsked: r.TierAsked, TierSent: r.TierSent, TierServed: r.TierServed,
			FirstReasoning: r.FirstReasoning, ReasoningHidden: r.ReasoningHidden, Own: r.Own, Retried: r.Retried, Queued: r.Queued,
			Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, CacheWrite: r.CacheWrite,
			// The WinOne provider ID differs from the upstream vendor ID, so the
			// explicit flag must survive rather than rely on legacy ID inference.
			CacheUnknown: r.CacheUnknown,
			CacheWrite1h: r.CacheWrite1h, Reasoning: r.Reasoning, Millis: r.Millis,
			TTFT: r.TTFT, FirstText: r.FirstText, Sent: r.Sent, Flow: r.Flow, Status: r.Status, Error: failure, Rejected: r.Rejected}
	}
	return nil
}

type usageQuery struct {
	Models []string `json:"models,omitempty"`
	// Use the viewer's IANA zone for the ledger, heatmap and session costs.
	Zone      string `json:"tz,omitempty"`
	CallerKey string `json:"callerKey,omitempty"`
	Period    string `json:"period"`
	Provider  string `json:"provider"`
	Source    string `json:"source"`
	Failed    bool   `json:"failed"`
	Offset    int    `json:"offset"`
	Day       string `json:"day"`
	Model     string `json:"model"`
	Account   string `json:"account"`
	Purpose   string `json:"purpose"`
	Query     string `json:"query"`
	Export    bool   `json:"export,omitempty"`
}

func (q usageQuery) filter() usage.Filter {
	// Through is fixed: upstream session discovery must never scan another
	// client's profile. WinOne Chat has its own registered-session reader.
	return usage.Filter{Zone: usage.Zone(q.Zone), CallerKey: q.CallerKey, Provider: q.Provider, Agent: q.Source, Failed: q.Failed, Through: usage.SourceGateway, Day: q.Day, Model: q.Model, Account: q.Account, Purpose: q.Purpose, Query: q.Query}
}

func readUsage(in request, output io.Writer) error {
	if err := configureUsage(in); err != nil {
		return err
	}
	// A source filter alone is too late: upstream enumerates session sources
	// before applying it. Use its injected reader to skip that discovery.
	priorLogCalls, priorProviders := usage.LogCalls, provider.WinOneProviders
	priorAlias := usage.WinOneModelAlias
	usage.WinOneModelAlias = provider.UsageAlias()
	usage.LogCalls = func(time.Time) []sessions.Call { return nil }
	provider.WinOneProviders = []provider.Provider{}
	defer func() {
		usage.LogCalls = priorLogCalls
		provider.WinOneProviders = priorProviders
		usage.WinOneModelAlias = priorAlias
	}()
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
	if len(q.Zone) > 128 || strings.ContainsRune(q.Zone, '\x00') || len(q.Model) > 512 || len(q.Account) > 256 || len(q.Purpose) > 128 || len(q.CallerKey) > 128 || len(q.Query) > 512 || strings.ContainsRune(q.Query, '\x00') {
		return errors.New("invalid usage filter")
	}
	modelAliases := make(map[string]string, len(q.Models))
	for _, model := range q.Models {
		if len(model) > 512 || strings.ContainsRune(model, '\x00') {
			return errors.New("invalid usage model")
		}
		modelAliases[model] = usage.WinOneModelGroup(model)
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
	// Chat supplies only its registered model IDs. Resolve them with the same
	// upstream function, including models that have no gateway ledger record.
	result["modelAliases"] = modelAliases
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
		p.Days[r.Time.In(usage.Zone(q.Zone)).Format("2006-01-02")] += r.Cost
	}
	result["sessionCosts"] = prices
	for _, r := range usage.Load(usage.Period(q.Period).Since(time.Now().In(usage.Zone(q.Zone)))) {
		if len(r.WinOnePrompt) == 0 || q.Provider != "" && q.Provider != r.Provider || q.Source != "" && q.Source != r.Agent || q.Model != "" && usage.WinOneModelGroup(q.Model) != usage.WinOneModelGroup(r.Model) {
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

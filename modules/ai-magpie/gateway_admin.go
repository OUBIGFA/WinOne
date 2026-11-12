package main

import (
	"cmp"
	"encoding/json"
	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/budget"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/oidc"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/usage"
	"net/http"
	"slices"
	"strings"
	"time"
)

func winOneGatewayAdminRoutes(mux *http.ServeMux) {
	// Only gateway-owned data: never list or read an external client's files.
	historyState := func() any {
		return map[string]any{"recording": settings.Load().GatewayConversations, "recorded": sessions.HasGatewayConversations(), "sessions": listedGateway(usage.GatewaySessions(time.Time{}, nil))}
	}
	mux.HandleFunc("GET /api/winone/gateway/conversations", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, historyState()) })
	mux.HandleFunc("POST /api/winone/gateway/conversations/recording", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ On, Clear bool }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
			http.Error(w, "Invalid recording choice", 400)
			return
		}
		if err := sessions.SetGatewayRecording(in.On, in.Clear); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, historyState())
	})
	mux.HandleFunc("POST /api/winone/gateway/conversations/read", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Agent, ID string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil {
			http.Error(w, "Invalid gateway session", 400)
			return
		}
		transcript, err := sessions.GatewayTranscript(in.Agent, in.ID)
		if err != nil {
			fail(w, err)
			return
		}
		var markdown strings.Builder
		if err := sessions.WriteGatewayMarkdown(&markdown, sessions.Session{ID: in.ID, Agent: in.Agent}, in.Agent, transcript); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"text": markdown.String(), "cut": transcript.Cut})
	})
	mux.HandleFunc("POST /api/winone/gateway/configure", func(w http.ResponseWriter, r *http.Request) {
		var in access.Key
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil {
			http.Error(w, "Invalid key policy", 400)
			return
		}
		if err := access.WinOneConfigure(in); err != nil {
			fail(w, err)
			return
		}
		keys, err := access.List()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"keys": withLimits(keys), "accountNames": provider.AccountNames()})
	})
	mux.HandleFunc("POST /api/winone/gateway/copy", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "Invalid key", 400)
			return
		}
		keys, err := access.Export()
		if err != nil {
			fail(w, err)
			return
		}
		for _, k := range keys {
			if k.ID == in.ID {
				writeJSON(w, map[string]string{"secret": k.Secret})
				return
			}
		}
		http.Error(w, "Key not found", 404)
	})
	mux.HandleFunc("POST /api/winone/gateway/issue", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name     string   `json:"name"`
			TTL      string   `json:"ttl"`
			Models   []string `json:"models"`
			Accounts []string `json:"accounts"`
			Tokens   int64    `json:"tokens"`
			Cost     float64  `json:"cost"`
			ReadOnly bool     `json:"readOnly"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "Invalid issued key", 400)
			return
		}
		ttl, err := time.ParseDuration(in.TTL)
		if err != nil {
			fail(w, err)
			return
		}
		key, err := access.IssueKey(access.Issue{Name: in.Name, TTL: ttl, Models: in.Models, Accounts: in.Accounts, Tokens: in.Tokens, Cost: in.Cost, ReadOnly: in.ReadOnly})
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, key)
	})
	mux.HandleFunc("GET /api/winone/gateway/oidc", func(w http.ResponseWriter, r *http.Request) {
		rules, err := oidc.List()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"rules": rules})
	})
	mux.HandleFunc("POST /api/winone/gateway/oidc", func(w http.ResponseWriter, r *http.Request) {
		var in oidc.Rule
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "Invalid identity rule", 400)
			return
		}
		rule, err := oidc.Add(in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, rule)
	})
	mux.HandleFunc("POST /api/winone/gateway/oidc/remove", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "Invalid identity rule", 400)
			return
		}
		if err := oidc.Remove(in.ID); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]bool{"removed": true})
	})
}

// Only retained native gateway operations are registered. A former desktop
// plugin or external-client URL is a 404 even with this private capability.
func gatewayAdminHandler(gw *gateway.Server) http.Handler {
	mux := http.NewServeMux()
	callerKeyRoutes(mux)
	winOneGatewayAdminRoutes(mux)
	winOneCallRoutes(mux, gw)
	mux.HandleFunc("GET /api/claude/mode", func(w http.ResponseWriter, r *http.Request) {
		mode, err := gateway.WinOneClaudeMode()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, mode)
	})
	mux.HandleFunc("GET /api/gateway/trace", func(w http.ResponseWriter, r *http.Request) {
		state := gw.Trace(r.Context(), 0, 0)
		// Pricing uses only these gateway calls; it never consults external
		// session directories, a global usage page or a second gateway owner.
		rows := make([]routeJSON, 0, len(state.Routes))
		price := usage.NewPricer()
		for _, route := range state.Routes {
			records := make([]usage.Record, 0, len(route.Usage))
			for _, u := range route.Usage {
				records = append(records, u.PricingRecord())
			}
			total := price(records)
			rows = append(rows, routeJSON{Route: route, Cost: total.Cost, Priced: total.Calls > total.Unpriced, Unpriced: total.Unpriced})
		}
		writeJSON(w, map[string]any{"routes": rows, "seq": state.Seq})
	})
	return mux
}

// Adapted from the pinned Magpie caller_keys.go and sessions_manage.go (MIT).
type keyRow struct {
	access.Key
	Used    *budget.Status  `json:"used,omitempty"`
	UsedAll []budget.Status `json:"usedAll,omitempty"`
}

type keyModelJSON struct {
	ID           string `json:"id"` // "<provider>/<model>", as stored by the key
	Name         string `json:"name"`
	Provider     string `json:"provider,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
	Account      bool   `json:"account,omitempty"` // a signed-in account the key may be held to (#905)
	Key          bool   `json:"key,omitempty"`     // one of a provider's keys, by its KeyID
	Plan         string `json:"plan,omitempty"`    // the account's subscription, for the menu's note
	Decides      bool   `json:"decides,omitempty"`
}

func withLimits(keys []access.Key) []keyRow {
	now := time.Now()
	out := make([]keyRow, 0, len(keys))
	for _, k := range keys {
		row := keyRow{Key: k, UsedAll: budget.All(k, now)}
		if len(row.UsedAll) > 0 {
			row.Used = &row.UsedAll[0]
		}
		out = append(out, row)
	}
	return out
}

func callerKeyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/caller-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		access.MigrateLegacyLANKeyBestEffort()
		keys, err := access.List()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"keys": withLimits(keys), "accountNames": provider.AccountNames()})
	})
	// Models, accounts and key fingerprints available to this gateway key.
	mux.HandleFunc("GET /api/caller-keys/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		out := []keyModelJSON{}
		accounts := []keyModelJSON{}
		for _, e := range provider.Catalog() {
			if e.Group != "" {
				continue
			}
			out = append(out, keyModelJSON{ID: e.Provider.ID + "/" + e.Model, Name: cmp.Or(e.Name, e.Model), Provider: e.Provider.ID, ProviderName: cmp.Or(e.Provider.Name, e.Provider.ID)})
		}
		// Keep decision models offered by enabled providers.
		for _, e := range provider.OfferedDeciders(nil) {
			id := e.Provider.ID + "/" + e.Model
			if slices.ContainsFunc(out, func(m keyModelJSON) bool { return strings.EqualFold(m.ID, id) }) {
				continue
			}
			m := keyModelJSON{ID: id, Name: cmp.Or(e.Name, e.Model), Provider: e.Provider.ID, ProviderName: cmp.Or(e.Provider.Name, e.Provider.ID), Decides: true}
			at := len(out)
			for i := len(out) - 1; i >= 0; i-- {
				if out[i].Provider == m.Provider {
					at = i + 1
					break
				}
			}
			out = slices.Insert(out, at, m)
		}
		for _, p := range provider.All() {
			if !p.On() {
				continue
			}
			name := cmp.Or(p.Name, p.ID)
			// an account by its stable id, kept through renames, shown by the
			// name the user gave it, else who is signed in; a key by its
			// fingerprint
			for _, a := range p.AccountIDs() {
				accounts = append(accounts, keyModelJSON{ID: p.ID + "/" + a.ID, Name: a.Shown(p), Provider: p.ID, ProviderName: name, Account: !a.Key, Key: a.Key, Plan: a.Plan})
			}
		}
		writeJSON(w, map[string]any{"models": out, "accounts": accounts})
	})
	mux.HandleFunc("POST /api/caller-keys/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in access.Change
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			fail(w, err)
			return
		}
		access.MigrateLegacyLANKeyBestEffort()
		secret, err := access.Update(r.PathValue("action"), in)
		if err != nil {
			fail(w, err)
			return
		}
		keys, err := access.List()
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, map[string]any{"keys": withLimits(keys), "accountNames": provider.AccountNames(), "secret": secret})
	})
}

func listedGateway(gs []usage.GatewaySession) []usage.GatewaySession {
	hidden := sessions.GatewayHidden()
	out := gs[:0:0]
	for _, g := range gs {
		if !hidden(g.Agent, g.ID, g.Last) {
			out = append(out, g)
		}
	}
	return out
}

type routeJSON struct {
	gateway.Route
	SessionTitle string  `json:"sessionTitle,omitempty"`
	Cost         float64 `json:"cost"`
	Priced       bool    `json:"priced"`
	Unpriced     int     `json:"unpriced"`
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]string{"error": err.Error()})
}

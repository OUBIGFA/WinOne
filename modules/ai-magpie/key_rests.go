package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// The private runtime owns the fingerprint-to-slot mapping. Neither the UI
// nor a public gateway caller receives vendor keys to derive this identity.
var gatewayKeySlots sync.Map

func rememberGatewayKeySlots(id string, in request) {
	slots := map[string]string{}
	for _, key := range in.Keys {
		slots[provider.KeyID(key.Key)] = key.ID
	}
	if in.Key != "" && in.KeyID != "" {
		slots[provider.KeyID(in.Key)] = in.KeyID
	}
	gatewayKeySlots.Store(id, slots)
}

type keyRestRow struct {
	ID   string            `json:"id"`
	Rest *provider.KeyRest `json:"rest,omitempty"`
}

func gatewayKeyRestState(p provider.Provider) []keyRestRow {
	slots, _ := gatewayKeySlots.Load(p.ID)
	byID, _ := slots.(map[string]string)
	rows := []keyRestRow{}
	restKey := p.RestKeys()
	for _, key := range p.KeyList() {
		id := byID[key.ID]
		if id == "" {
			continue
		}
		row := keyRestRow{ID: id}
		if r, ok := gateway.RestOf(restKey(key)); ok && key.On {
			row.Rest = protectedKeyRest(p, r)
		}
		rows = append(rows, row)
	}
	return rows
}

// Vendor reasons may echo credentials. Protect the complete saved key set
// before this live state crosses the private Host boundary.
func protectedKeyRest(p provider.Provider, r gateway.Rest) *provider.KeyRest {
	said := r.Error
	secrets := []string{p.Key, p.AccessKeyID, p.SecretAccessKey, p.BalanceToken}
	for _, key := range p.Keys {
		secrets = append(secrets, key.Key)
	}
	for _, value := range p.Headers {
		secrets = append(secrets, value)
	}
	for _, secret := range secrets {
		if secret != "" {
			said = strings.ReplaceAll(said, secret, "[redacted]")
		}
	}
	return &provider.KeyRest{Why: r.Why, Status: r.Status, Until: r.Until, Probed: r.Probed, By: r.By, Error: said, Rate: r.Rate}
}

func winOneKeyRestRoutes(mux *http.ServeMux, gw *gateway.Server) {
	find := func(w http.ResponseWriter, r *http.Request) (*provider.Provider, bool) {
		p, err := provider.Find(r.PathValue("provider"))
		if err != nil || p.Account != nil || gateway.WinOneMiddlewareAccounts[p.ID] != "" {
			http.Error(w, "API-key provider not found", http.StatusNotFound)
			return nil, false
		}
		return p, true
	}
	mux.HandleFunc("GET /api/winone/providers/{provider}/key-rests", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := find(w, r); ok {
			writeJSON(w, map[string]any{"keys": gatewayKeyRestState(*p)})
		}
	})
	mux.HandleFunc("POST /api/winone/providers/{provider}/key-rests/retry", func(w http.ResponseWriter, r *http.Request) {
		p, ok := find(w, r)
		if !ok {
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
			http.Error(w, "Invalid key slot", 400)
			return
		}
		slots, _ := gatewayKeySlots.Load(p.ID)
		byID, _ := slots.(map[string]string)
		for _, key := range p.KeyList() {
			if key.On && byID[key.ID] == in.ID && in.ID != "" {
				// Lift the original runtime's rest; no second cooldown or retry
				// queue is created, and the next real request checks the vendor.
				gw.Unrest(p.RestKey(key))
				writeJSON(w, map[string]any{"keys": gatewayKeyRestState(*p)})
				return
			}
		}
		http.Error(w, "Key slot not found", 404)
	})
}

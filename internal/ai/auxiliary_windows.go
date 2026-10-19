//go:build windows

package ai

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *Store) runtimeProvidersLocked(c Configuration) ([]RuntimeProvider, error) {
	result := []RuntimeProvider{}
	for _, p := range c.Providers {
		if len(p.Models) == 0 || p.AuthMode == "account" && c.MagpieSettings.NoAccountDiscovery {
			continue
		}
		var key string
		var keys []RuntimeKey
		if p.AuthMode == "api-key" {
			var err error
			plan := s.keyPlanLocked(p)
			key, err = plan.firstKey()
			if err != nil {
				continue
			}
			for _, credential := range plan.keys {
				secret, err := unsealKey(credential.sealed)
				if err != nil {
					return nil, err
				}
				keys = append(keys, RuntimeKey{ID: credential.id, Key: secret})
			}
		}
		result = append(result, RuntimeProvider{Provider: p, Key: key, Keys: keys})
	}
	return result, nil
}

// MagpieProviders is a private snapshot for catalog/auxiliary execution. Like
// MagpieRuntime, it must never be serialized to an IPC settings response.
func (s *Store) MagpieProviders() ([]RuntimeProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeProvidersLocked(s.configurationLocked())
}

// An ephemeral authenticated loopback bridge lets Magpie select any WinOne
// provider for image understanding, generation or search. Account homes stay
// separate: a helper reuses the normal short-lived Translator for its profile.
func (t Translator) auxiliaryBridge(ctx context.Context, providers []RuntimeProvider, prefs *MagpieSettings) ([]auxiliaryProvider, func(), error) {
	stop := func() {}
	if len(providers) == 0 {
		return nil, stop, nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, stop, err
	}
	var tokenBytes [32]byte
	if _, err = rand.Read(tokenBytes[:]); err != nil {
		listener.Close()
		return nil, stop, err
	}
	token := hex.EncodeToString(tokenBytes[:])
	base := "http://" + listener.Addr().String()
	byID := map[string]RuntimeProvider{}
	out := []auxiliaryProvider{}
	for _, p := range providers {
		byID[p.Provider.ID] = p
		url := base + "/" + p.Provider.ID
		helper := auxiliaryProvider{ID: p.Provider.ID, Name: p.Provider.Name, Icon: p.Provider.DisplayIcon(), Chat: url + "/v1", Responses: url + "/v1", Anthropic: url, Key: token, Models: slices.Clone(p.Provider.Models), Metadata: p.Provider.ModelMetadata, Searches: p.Provider.Searches}
		if p.Provider.AuthMode == "account" {
			helper.AccountKind = p.Provider.Agent
		}
		// Give Magpie a distinct private bridge key for each enabled credential.
		// It can then retain its original key ordering, rotation and cooldowns.
		for index, key := range p.Keys {
			if index == 0 {
				helper.Key = token + ":" + key.ID
				helper.KeyName = key.ID
			} else {
				helper.Keys = append(helper.Keys, auxiliaryKey{Name: key.ID, Key: token + ":" + key.ID})
			}
		}
		out = append(out, helper)
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if supplied == "" {
			supplied = r.Header.Get("x-api-key")
		}
		bridgeToken, slot, _ := strings.Cut(supplied, ":")
		if subtle.ConstantTimeCompare([]byte(bridgeToken), []byte(token)) != 1 {
			GatewayError(w, 401, "辅助模型访问密钥无效")
			return
		}
		id, path, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		target, known := byID[id]
		if !ok || !known || r.Method != http.MethodPost {
			GatewayError(w, 404, "辅助模型不存在")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 48<<20))
		if err != nil {
			GatewayError(w, 400, "辅助请求无效")
			return
		}
		var envelope struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &envelope) != nil || !slices.Contains(target.Provider.Models, envelope.Model) {
			GatewayError(w, 400, "辅助模型未启用")
			return
		}
		p := target.Provider
		key := target.Key
		if slot != "" {
			found := false
			for _, credential := range target.Keys {
				if credential.ID == slot {
					key = credential.Key
					found = true
					break
				}
			}
			if !found {
				GatewayError(w, 403, "辅助密钥未启用")
				return
			}
		}
		inner := Preferences{ProviderID: p.ID, ProviderName: p.Name, Protocol: p.Protocol, BaseURL: p.BaseURL, Model: envelope.Model, Agent: p.Agent, AgentBackend: p.AgentBackend, AuthMode: p.AuthMode, Preset: p.Preset, PresetRegion: p.PresetRegion, Headers: p.Headers, ModelMetadata: p.ModelMetadataFor(envelope.Model), RequestLimits: p.RequestLimits, MagpieSettings: prefs.clone()}
		// The outer gateway already chose a helper; a helper cannot recursively
		// add more helpers, and its quota belongs to the original held request.
		inner.MagpieSettings.Vision = "off"
		inner.MagpieSettings.ImageGen = "off"
		inner.MagpieSettings.Searcher = "off"
		root := t
		// The authenticated outer gateway has already applied user hooks and
		// middleware. This worker only transports its chosen provider request.
		root.TransportOnly = true
		if root.RootDataDirectory != "" {
			root.DataDirectory = root.RootDataDirectory
		}
		partial, err := root.ForProvider(p.ID).ChatRequest(r.Context(), inner, key, "/"+path, body, r.Header, w)
		if err != nil && !partial {
			status := 502
			var failed *AttemptFailure
			if errors.As(err, &failed) {
				status = failed.Status
				if failed.RetryAfter > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(int(failed.RetryAfter/time.Second)))
				}
			}
			GatewayError(w, status, "辅助模型请求失败")
		}
	})
	go server.Serve(listener)
	stop = func() { server.Close() }
	return out, stop, nil
}

//go:build windows

package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"sync"
	"time"
)

func (s *Store) runtimeProvidersLocked(c Configuration) ([]RuntimeProvider, error) {
	result := []RuntimeProvider{}
	for _, p := range c.Providers {
		p.Models = slices.DeleteFunc(slices.Clone(p.Models), func(model string) bool {
			return c.MagpieSettings.ModelDisabled(p.ID, model, p.ModelMetadata[model].Alias)
		})
		if p.Off || len(p.Models) == 0 || p.AuthMode == "account" && c.MagpieSettings.NoAccountDiscovery {
			continue
		}
		if err := s.hydrateProviderSecrets(&p); err != nil {
			return nil, err
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
				keys = append(keys, RuntimeKey{ID: credential.id, Key: secret, KeyOptions: credential.options})
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
	return t.auxiliaryBridgeCatalog(ctx, providers, prefs, nil)
}

// A running router can replace model catalogs without replacing its credentials
// or canceling requests. Each attempt takes one immutable provider snapshot.
func (t Translator) auxiliaryBridgeCatalog(ctx context.Context, providers []RuntimeProvider, prefs *MagpieSettings, update *func(Provider)) ([]auxiliaryProvider, func(), error) {
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
	var catalogMu sync.RWMutex
	if update != nil {
		*update = func(p Provider) {
			catalogMu.Lock()
			defer catalogMu.Unlock()
			if current, ok := byID[p.ID]; ok {
				// A call selected before the refresh may reach this bridge later.
				// Retain its transport until this process exits; only the outer
				// gateway publishes/selects the newly enabled catalog.
				models := slices.Clone(p.Models)
				for _, id := range current.Provider.Models {
					if !slices.Contains(models, id) {
						models = append(models, id)
					}
				}
				metadata := make(map[string]ModelMetadata, len(current.Provider.ModelMetadata)+len(p.ModelMetadata))
				for id, m := range current.Provider.ModelMetadata {
					metadata[id] = m
				}
				for id, m := range p.ModelMetadata {
					metadata[id] = m
				}
				current.Provider.Models = models
				current.Provider.ModelMetadata = metadata
				current.Provider.Contexts = p.Contexts
				byID[p.ID] = current
			}
		}
	}
	out := []auxiliaryProvider{}
	for _, p := range providers {
		byID[p.Provider.ID] = p
		url := base + "/" + p.Provider.ID
		helper := auxiliaryProvider{ProviderPreferences: p.Provider.ProviderPreferences, ID: p.Provider.ID, Name: p.Provider.Name, Icon: p.Provider.DisplayIcon(), Chat: url + "/v1", Responses: url + "/v1", Anthropic: url, Key: token, Models: slices.Clone(p.Provider.Models), Metadata: p.Provider.ModelMetadata, Searches: p.Provider.Searches, LimitRestOff: p.Provider.LimitRestOff}
		helper.ModelNames = p.Provider.ModelNames
		// The outer router talks only to its authenticated local bridge.
		// Transport choices apply once, to the vendor, in the inner worker.
		helper.Proxy, helper.AuthHeader, helper.ExtraBody = "direct", "", nil
		// Supplemental credentials and vendor discovery URLs belong solely to
		// the inner worker, never the outer router's loopback provider.
		helper.AccessKeyID, helper.SecretAccessKey, helper.BalanceToken = nil, nil, nil
		helper.ModelsURL, helper.BalanceURL = "", ""
		helper.Vertex, helper.ZhipuTeam, helper.Endpoints = nil, nil, nil
		helper.KeySpendCaps, helper.KeyOwnBalances = nil, nil
		helper.InsecureTLS = false
		if p.Provider.AuthMode == "api-key" {
			helper.DecisionSource = &decisionSource{Protocol: p.Provider.Protocol, BaseURL: p.Provider.BaseURL, Preset: p.Provider.Preset, PresetRegion: p.Provider.PresetRegion, ProviderPreferences: &ProviderPreferences{Endpoints: p.Provider.Endpoints}}
			// Native APIs execute inside the same upstream gateway so its unknown-
			// path, privacy and key-scope guards remain authoritative. This private
			// process payload never enters settings responses or the model catalog.
			helper.NativeSource = map[string]any{"providerId": p.Provider.ID, "protocol": p.Provider.Protocol, "baseUrl": p.Provider.BaseURL, "preset": p.Provider.Preset, "presetRegion": p.Provider.PresetRegion, "providerPreferences": p.Provider.ProviderPreferences, "headers": p.Provider.Headers, "key": p.Key}
			if len(p.Keys) > 0 {
				helper.NativeSource["key"], helper.NativeSource["keyID"], helper.NativeSource["keyOptions"] = p.Keys[0].Key, p.Keys[0].ID, p.Keys[0].KeyOptions
				for _, key := range p.Keys[1:] {
					helper.NativeKeys = append(helper.NativeKeys, auxiliaryKey{Name: key.Name, Key: key.Key, KeyOptions: key.KeyOptions})
				}
			}
		}
		if p.Provider.AuthMode == "account" {
			helper.AccountKind = p.Provider.Agent
		}
		// Give Magpie a distinct private bridge key for each enabled credential.
		// It can then retain its original key ordering, rotation and cooldowns.
		for index, key := range p.Keys {
			// Match Magpie's public KeyID fingerprint so restarting a private
			// bridge does not reset a key's spending or its recorded balance.
			digest := sha256.Sum256([]byte(key.Key))
			fingerprint := hex.EncodeToString(digest[:5])
			bridgeKey := token + ":" + key.ID
			if helper.KeyIDs == nil {
				helper.KeyIDs = map[string]string{}
			}
			helper.KeyIDs[bridgeKey] = fingerprint
			if cap, ok := p.Provider.KeySpendCaps[key.ID]; ok {
				if helper.KeySpendCaps == nil {
					helper.KeySpendCaps = map[string]SpendCap{}
				}
				helper.KeySpendCaps[fingerprint] = cap
			}
			if balance, ok := p.Provider.KeyOwnBalances[key.ID]; ok {
				if helper.KeyOwnBalances == nil {
					helper.KeyOwnBalances = map[string]OwnBalance{}
				}
				helper.KeyOwnBalances[fingerprint] = balance
			}
			name := key.Name
			if name == "" {
				name = key.ID
			}
			if index == 0 {
				helper.Key = bridgeKey
				helper.KeyName = name
				helper.KeyProtocol, helper.KeyWeight = key.Protocol, key.Weight
				helper.KeyPlan = p.Provider.KeyPlan
				if key.Plan != "" {
					helper.KeyPlan = key.Plan
				}
			} else {
				helper.Keys = append(helper.Keys, auxiliaryKey{Name: name, Key: bridgeKey, KeyOptions: key.KeyOptions})
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
		catalogMu.RLock()
		target, known := byID[id]
		catalogMu.RUnlock()
		videoRead := r.Method == http.MethodGet && strings.HasPrefix(path, "v1/videos/")
		if !ok || !known || r.Method != http.MethodPost && !videoRead {
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
		if videoRead {
			// The opaque video id and original enabled provider bind the read;
			// GETs have no model body and never create a new vendor operation.
			if len(target.Provider.Models) == 0 {
				GatewayError(w, 404, "辅助模型未启用")
				return
			}
			envelope.Model = target.Provider.Models[0]
		} else if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			parsed := r.Clone(r.Context())
			parsed.Body = io.NopCloser(strings.NewReader(string(body)))
			if err := parsed.ParseMultipartForm(48 << 20); err != nil {
				GatewayError(w, 400, "辅助图片请求无效")
				return
			}
			if parsed.MultipartForm != nil {
				defer parsed.MultipartForm.RemoveAll()
			}
			envelope.Model = parsed.FormValue("model")
			if !slices.Contains(target.Provider.Models, envelope.Model) {
				GatewayError(w, 400, "辅助模型未启用")
				return
			}
		} else if json.Unmarshal(body, &envelope) != nil || !slices.Contains(target.Provider.Models, envelope.Model) {
			GatewayError(w, 400, "辅助模型未启用")
			return
		}
		p := target.Provider
		key := target.Key
		requestContext := r.Context()
		if slot != "" {
			found := false
			for _, credential := range target.Keys {
				if credential.ID == slot {
					key = credential.Key
					requestContext = contextWithRuntimeKey(requestContext, credential)
					found = true
					break
				}
			}
			if !found {
				GatewayError(w, 403, "辅助密钥未启用")
				return
			}
		}
		inner := Preferences{ProviderPreferences: p.ProviderPreferences, ProviderID: p.ID, ProviderName: p.Name, Protocol: p.Protocol, BaseURL: p.BaseURL, Model: envelope.Model, Agent: p.Agent, AgentBackend: p.AgentBackend, AuthMode: p.AuthMode, Preset: p.Preset, PresetRegion: p.PresetRegion, LimitRestOff: p.LimitRestOff, AdditionalTools: p.AdditionalTools, Headers: p.Headers, ModelMetadata: p.ModelMetadataFor(envelope.Model), RequestLimits: p.RequestLimits, MagpieSettings: prefs.clone()}
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
		partial, err := root.ForProvider(p.ID).ChatRequest(requestContext, inner, key, "/"+path, body, r.Header, w, r.Method)
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

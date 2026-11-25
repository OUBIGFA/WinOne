package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/netproxy"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"math"
	"net/url"
	"slices"
	"strings"
)

// Match the Host bound without discarding upstream catalog entries.
const maximumCatalogModels = 4096

// Keep the saved provider ID through protocol adaptation, model preferences,
// fallbacks and disabled-model rules. Legacy callers retain their old ID.
func requestProviderID(in request) string {
	if in.ProviderID != "" {
		return in.ProviderID
	}
	return "winone"
}

type modelOption struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Free     bool          `json:"free,omitempty"`
	Metadata modelMetadata `json:"metadata,omitempty"`
}

// The host owns persistence. Worker options mirror Magpie's model window,
// output and effort semantics without writing agents' configuration files.
type modelMetadata struct {
	Draws        bool              `json:"draws,omitempty"`
	Films        bool              `json:"films,omitempty"`
	Serves       string            `json:"serves,omitempty"`
	Alias        string            `json:"alias,omitempty"`
	Searches     bool              `json:"searches,omitempty"`
	Same         string            `json:"same,omitempty"`
	UpstreamName string            `json:"upstreamName,omitempty"`
	API          string            `json:"api,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Keys         []string          `json:"keys,omitempty"`
	KeyCount     int               `json:"keyCount,omitempty"`
	// Levels are the vendor's reported reasoning levels; Efforts the offered
	// (kept) ones. Both are the vendor's own words, never a fixed ladder.
	Levels  []string       `json:"levels,omitempty"`
	Efforts []string       `json:"efforts,omitempty"`
	Tiers   []string       `json:"tiers,omitempty"`
	Price   *catalog.Price `json:"price,omitempty"`
	// Keep the vendor's current and pre-discount credit rates for model tags.
	Rate                float64 `json:"rate,omitempty"`
	RateWas             float64 `json:"rateWas,omitempty"`
	ContextWindow       int     `json:"contextWindow,omitempty"`
	MaxTokens           int     `json:"maxTokens,omitempty"`
	CompactionThreshold int     `json:"compactionThreshold,omitempty"`
	CompactionPercent   float64 `json:"compactionPercent,omitempty"`
	ListedContextWindow int     `json:"listedContextWindow,omitempty"`
	MaxContextWindow    int     `json:"maxContextWindow,omitempty"`
	Reasoning           *bool   `json:"reasoning,omitempty"`
	Vision              *bool   `json:"vision,omitempty"`
	Tools               *bool   `json:"tools,omitempty"`
	ReasoningEffort     string  `json:"reasoningEffort,omitempty"`
	Tier                string  `json:"tier,omitempty"`
}

// Magpie exposes output/compaction overrides through Settings, not fields on
// Provider. Use a request-local snapshot to interpret that public contract;
// never persist runtime overrides into another client's settings files.
func modelLimitSnapshot(p provider.Provider, model string, m modelMetadata, window int) settings.Settings {
	snapshot := settings.Settings{}
	if m.MaxTokens > 0 {
		snapshot.ModelOutputs = map[string]int{p.ID + "/" + model: m.MaxTokens}
	}
	threshold := m.CompactionThreshold
	if m.CompactionPercent > 0 && window > 0 {
		threshold = int(math.Floor(float64(window) * m.CompactionPercent / 100))
	}
	if threshold > 0 {
		snapshot.ModelCompacts = map[string]int{p.ID + "/" + model: threshold}
	}
	return snapshot
}

func describeModel(p provider.Provider, model catalog.Model, override modelMetadata) modelMetadata {
	window := p.WindowOf(model)
	if override.ContextWindow > 0 {
		window = override.ContextWindow
	}
	snapshot := modelLimitSnapshot(p, model.ID, override, window)
	// Current Magpie accepts a model entry so user-typed windows and explicit
	// thresholds share the same compaction policy used by its native catalog.
	threshold := provider.CompactFor(snapshot, provider.Entry{ID: p.ID + "/" + model.ID, Context: window, WindowSet: override.ContextWindow > 0 || p.ContextOf(model.ID) > 0}, nil)
	if threshold == 0 && override.CompactionPercent <= 0 && override.CompactionThreshold <= 0 {
		threshold = model.Compact
	}
	metadata := modelMetadata{Draws: model.Draws, Films: model.Films, Serves: model.Serves, Searches: model.WebSearch == "native", Levels: slices.Clone(model.Efforts), Efforts: slices.Clone(model.Efforts), Tiers: model.Tiers, Price: model.Price, Rate: model.Rate, RateWas: model.RateWas, ContextWindow: window, ListedContextWindow: provider.ListedWindow(model), MaxContextWindow: model.MaxContext, MaxTokens: provider.OutputWithin(window, p.ReplyLimitIn(model, snapshot)), CompactionThreshold: threshold, CompactionPercent: override.CompactionPercent, Vision: model.ImageInput}
	if model.Reasoning || len(model.Efforts) > 0 {
		yes := true
		metadata.Reasoning = &yes
	}
	if model.Images {
		yes := true
		metadata.Vision = &yes
	}
	return metadata
}

// Generation and discovery use the same magpie provider and endpoint checks.
// No network, credential or model-discovery stack is duplicated in WinOne UI.
func configuredProvider(in request) (provider.Provider, error) {
	base, err := url.Parse(in.BaseURL)
	// Match the Host: an explicitly configured HTTP relay is supported beyond
	// loopback too. Do not silently rewrite it to HTTPS or another destination.
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "https" && base.Scheme != "http") {
		return provider.Provider{}, errors.New("AI endpoint must be an HTTP or HTTPS base URL without credentials, query or fragment")
	}
	p := provider.Provider{ID: requestProviderID(in), Name: "WinOne", Key: in.Key}
	if in.Preset != "" {
		p, err = provider.FromPreset(in.Preset)
		if err != nil {
			return provider.Provider{}, errors.New("unsupported AI service preset")
		}
		if in.PresetRegion != "" {
			found := false
			for _, region := range provider.Preset(in.Preset).Regions {
				if region.ID != in.PresetRegion {
					continue
				}
				found = true
				p.Chat, p.Responses, p.Anthropic, p.Decide = region.Chat, region.Responses, region.Anthropic, region.Decide
				if region.Catalog != "" {
					p.Catalog = region.Catalog
				}
				break
			}
			if !found {
				return provider.Provider{}, errors.New("unsupported service region")
			}
		}
		expected := p.Chat
		if in.Protocol == "responses" {
			expected = p.Responses
		} else if in.Protocol == "anthropic" {
			expected = p.Anthropic
		} else if in.Protocol == "gemini" {
			expected = p.Gemini
		} else if in.Protocol == "decide" {
			expected = p.Decide
		}
		// An edited endpoint is explicit: do not retain other vendor URLs as
		// invisible fallbacks. An unchanged preset keeps its native protocols.
		if strings.TrimRight(expected, "/") != strings.TrimRight(in.BaseURL, "/") {
			p.Chat, p.Responses, p.Anthropic, p.Gemini, p.Decide = "", "", "", "", ""
		}
		p.ID, p.Key = requestProviderID(in), in.Key
		p.BaseAPI = in.Protocol
	}
	if in.Model != "" {
		p.Models = []string{in.Model}
		if in.ModelMetadata.ContextWindow > 0 {
			p.Contexts = map[string]int{in.Model: in.ModelMetadata.ContextWindow}
		}
	}
	p.AdditionalTools = in.AdditionalTools
	p.LimitRestOff = in.LimitRestOff
	if err := applyProviderPreferences(&p, in); err != nil {
		return p, err
	}
	if in.Model != "" && in.ModelMetadata.ContextWindow > 0 {
		if p.Contexts == nil {
			p.Contexts = map[string]int{}
		}
		p.Contexts[in.Model] = in.ModelMetadata.ContextWindow
	}
	p.MaxRPM, p.QueueLimit, p.QueueWait = in.RequestLimits.MaxRPM, in.RequestLimits.QueueLimit, in.RequestLimits.QueueWait
	if in.RequestLimits.MaxConcurrency != nil {
		p.MaxConcurrency = in.RequestLimits.MaxConcurrency
	}
	if in.ProviderPreferences != nil && in.ProviderPreferences.Endpoints != nil {
		e := in.ProviderPreferences.Endpoints
		p.Chat, p.Responses, p.Anthropic, p.Gemini, p.Decide = e.Chat, e.Responses, e.Anthropic, e.Gemini, e.Decide
		// An explicitly cleared Gemini URL must stay clear in upstream normalize.
		p.GeminiFilled = true
	}
	p.BaseAPI = in.Protocol
	if k := in.KeyOptions; k != nil {
		p.KeyName, p.KeyProtocol, p.KeyWeight = k.Name, provider.Protocol(k.Protocol), k.Weight
		if k.Plan != "" {
			p.KeyPlan = k.Plan
		}
	}
	if in.KeyID != "" && p.Key != "" {
		fingerprint := provider.KeyID(p.Key)
		if cap, ok := p.KeySpendCaps[in.KeyID]; ok {
			p.KeySpendCaps = map[string]provider.SpendCap{fingerprint: cap}
		}
		if balance, ok := p.KeyOwnBalances[in.KeyID]; ok {
			p.KeyOwnBalances = map[string]provider.OwnBalance{fingerprint: balance}
		}
		if rate, ok := p.KeyPriceRates[in.KeyID]; ok {
			p.KeyPriceRates = map[string]float64{fingerprint: rate}
		}
		if factor, ok := p.KeyRateFactors[in.KeyID]; ok {
			p.KeyRateFactors = map[string]float64{fingerprint: factor}
		}
	}
	if len(in.Keys) > 0 {
		// The upstream provider receives the whole enabled key set. Its own
		// selection, weights, protocol support, budgets and failover all apply.
		p.Keys = nil
		caps, balances, rates, factors := p.KeySpendCaps, p.KeyOwnBalances, p.KeyPriceRates, p.KeyRateFactors
		if in.ProviderPreferences != nil {
			caps, balances, rates, factors = nil, nil, nil, nil
		}
		for i, key := range in.Keys {
			if i == 0 {
				p.Key, p.KeyName, p.KeyProtocol, p.KeyWeight = key.Key, key.Name, key.Protocol, key.Weight
				if key.Plan != "" {
					p.KeyPlan = key.Plan
				}
			} else {
				p.Keys = append(p.Keys, key.KeyAccount)
			}
			if options := in.ProviderPreferences; options != nil {
				fingerprint := provider.KeyID(key.Key)
				if v, ok := options.KeySpendCaps[key.ID]; ok {
					if caps == nil {
						caps = map[string]provider.SpendCap{}
					}
					caps[fingerprint] = provider.SpendCap(v)
				}
				if v, ok := options.KeyOwnBalances[key.ID]; ok {
					if balances == nil {
						balances = map[string]provider.OwnBalance{}
					}
					encoded, _ := json.Marshal(v)
					var balance provider.OwnBalance
					_ = json.Unmarshal(encoded, &balance)
					balances[fingerprint] = balance
				}
				if v, ok := options.KeyPriceRates[key.ID]; ok {
					if rates == nil {
						rates = map[string]float64{}
					}
					rates[fingerprint] = v
				}
				if v, ok := options.KeyRateFactors[key.ID]; ok {
					if factors == nil {
						factors = map[string]float64{}
					}
					factors[fingerprint] = v
				}
			}
		}
		p.KeySpendCaps, p.KeyOwnBalances, p.KeyPriceRates, p.KeyRateFactors = caps, balances, rates, factors
	}
	// Magpie applies these after protocol authentication, preserving names and
	// merging list-valued headers through the same path as its provider editor.
	for name, value := range in.Headers {
		if p.Headers == nil {
			p.Headers = map[string]string{}
		}
		p.Headers[name] = value
	}
	switch in.Protocol {
	case "chat", "":
		p.Chat = strings.TrimRight(in.BaseURL, "/")
	case "responses":
		p.Responses = strings.TrimRight(in.BaseURL, "/")
	case "anthropic":
		p.Anthropic = strings.TrimRight(in.BaseURL, "/")
	case "gemini":
		p.Gemini = strings.TrimRight(in.BaseURL, "/")
	case "decide":
		p.Decide = strings.TrimRight(in.BaseURL, "/")
	default:
		return p, errors.New("unsupported AI protocol")
	}
	if in.ProviderPreferences == nil || in.ProviderPreferences.Endpoints == nil {
		// OpenCode's official endpoint migration also applies to discovery and
		// unsaved tests; explicit independent endpoints retain their own values.
		p = provider.WinOneOpenCodeEndpoints(p)
	}
	// Validate only after all actual endpoints have been applied. These are
	// upstream's own checks; never persist a provider while discovering/testing it.
	if len(p.Resolve) > 0 {
		if !p.TakesResolve() {
			return p, errors.New("connection rules require a provider with its own endpoint")
		}
		p.Resolve, err = netproxy.CheckResolve(p.Resolve)
		if err != nil {
			return p, err
		}
	}
	if p.ServerPin != "" {
		p.ServerPin = netproxy.NormalPin(p.ServerPin)
		if p.ServerPin == "" || !p.TakesServerPin() {
			return p, errors.New("a server certificate requires an HTTPS endpoint and its SHA-256 fingerprint")
		}
	}
	if p.ClientCert != "" || p.ClientKey != "" || p.ServerCA != "" {
		if !p.TakesClientCert() {
			return p, errors.New("client certificates require a provider with its own endpoint")
		}
		for _, file := range []*string{&p.ClientCert, &p.ClientKey, &p.ServerCA} {
			*file, err = provider.CertPath(*file)
			if err != nil {
				return p, err
			}
		}
		if err = netproxy.CheckClientCert(p.ClientCert, p.ClientKey, p.ServerCA); err != nil {
			return p, err
		}
	}
	return p, nil
}
func fetchModels(ctx context.Context, in request) ([]modelOption, error) {
	var p provider.Provider
	var err error
	if in.Agent != "" {
		account, findErr := findAccount(ctx, in.Agent)
		if findErr != nil {
			return nil, findErr
		}
		p = *account
		p.LimitRestOff = in.LimitRestOff
		if err := applyProviderPreferences(&p, in); err != nil {
			return nil, err
		}
	} else {
		p, err = configuredProvider(in)
		if err != nil {
			return nil, err
		}
		provider.WinOneProviders = []provider.Provider{p}
		p = provider.All()[0]
	}
	ctx, observed := observeFailures(ctx)
	// ListOff uses the user's explicit models and makes no discovery request.
	p.Models = slices.Clone(in.Models)
	models, err := p.Fetch(ctx)
	if err != nil {
		if failure := observed.result(); failure != nil && in.Agent == "" {
			return nil, failure
		}
		return nil, errors.New("model discovery failed; models can be entered manually")
	}
	// Fetch returns chat models; its original live catalog also retains image,
	// video, audio, embedding and rerank models. Keep the entire upstream list
	// available for the matching gateway APIs instead of losing those entries.
	if full, ok := catalog.LiveAll(p.ID); ok && p.Account == nil {
		models = full
	}
	extra := append(p.PlanDrawers(), p.PlanVideos()...)
	extra = append(extra, p.PlanSpeakers()...)
	extra = append(extra, p.PlanTranscribers()...)
	extra = append(extra, p.PlanRetrieval("")...)
	if p.Account == nil && !p.ListsOff() && len(extra) > 0 {
		// A plan's media are defined outside its chat /models reply. Use
		// the same lists as the native gateway, preserving their API kind.
		models = append(slices.Clone(models), extra...)
		unique := make([]catalog.Model, 0, len(models))
		seen := map[string]bool{}
		for _, model := range models {
			if !seen[model.ID] {
				unique = append(unique, model)
				seen[model.ID] = true
			}
		}
		models = unique
		// The shared router reads this original catalog rather than trying
		// to recreate media capabilities from the public display metadata.
		if err := catalog.WinOneSaveMediaList(p.ID, models); err != nil {
			return nil, err
		}
	}
	if p.Account != nil && in.ProviderID != "" && len(models) > 0 {
		// Account providers own private profiles. The outer gateway reads this
		// upstream-format snapshot without opening or copying account secrets.
		if err := catalog.SaveLive(in.ProviderID, "", models); err != nil {
			return nil, err
		}
	}
	if len(models) > maximumCatalogModels {
		return nil, errors.New("model catalog is too large")
	}
	result := []modelOption{}
	seen := map[string]bool{}
	for _, model := range models {
		if model.ID == "" || len(model.ID) > 256 || strings.TrimSpace(model.ID) != model.ID {
			return nil, errors.New("invalid model catalog")
		}
		if !seen[model.ID] {
			override := modelMetadata{}
			if model.ID == in.Model {
				override = in.ModelMetadata
			}
			metadata := describeModel(p, model, override)
			var onKeys []string
			positions := map[string]int{}
			slots := map[string]string{}
			for _, key := range p.KeysOn() {
				fingerprint := provider.KeyID(key.Key)
				positions[fingerprint] = len(onKeys)
				onKeys = append(onKeys, fingerprint)
			}
			for _, key := range in.Keys {
				slots[provider.KeyID(key.Key)] = key.ID
			}
			ids, count := keysApart(model.Keys, onKeys, positions)
			for _, id := range ids {
				if slot := slots[id]; slot != "" {
					metadata.Keys = append(metadata.Keys, slot)
				}
			}
			metadata.KeyCount = count
			result = append(result, modelOption{ID: model.ID, Name: model.Name, Free: model.Free, Metadata: metadata})
			seen[model.ID] = true
		}
	}
	return result, nil
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"math"
	"net/url"
	"strings"
)

// Match the Host bound without discarding upstream catalog entries.
const maximumCatalogModels = 4096

type modelOption struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Free     bool          `json:"free,omitempty"`
	Metadata modelMetadata `json:"metadata,omitempty"`
}

// The host owns persistence. Worker options mirror Magpie's model window,
// output and effort semantics without writing agents' configuration files.
type modelMetadata struct {
	Same         string         `json:"same,omitempty"`
	UpstreamName string         `json:"upstreamName,omitempty"`
	API          string         `json:"api,omitempty"`
	Efforts      []string       `json:"efforts,omitempty"`
	Tiers        []string       `json:"tiers,omitempty"`
	Price        *catalog.Price `json:"price,omitempty"`
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
	metadata := modelMetadata{Efforts: model.Efforts, Tiers: model.Tiers, Price: model.Price, Rate: model.Rate, RateWas: model.RateWas, ContextWindow: window, ListedContextWindow: provider.ListedWindow(model), MaxContextWindow: model.MaxContext, MaxTokens: p.ReplyLimitIn(model, snapshot), CompactionThreshold: threshold, CompactionPercent: override.CompactionPercent, Vision: model.ImageInput}
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

// Apply choices to the incoming protocol, then let Magpie translate them for
// the configured vendor. Explicit smaller client output limits stay smaller.
func applyModelMetadata(body map[string]any, path string, m modelMetadata) {
	output := "max_completion_tokens"
	if strings.Contains(path, "responses") {
		output = "max_output_tokens"
	} else if strings.Contains(path, "messages") {
		output = "max_tokens"
	}
	if m.MaxTokens > 0 {
		limit := m.MaxTokens
		for _, key := range []string{output, "max_tokens"} {
			if value, ok := body[key].(float64); ok && value > 0 && value < float64(limit) {
				limit = int(value)
			}
		}
		body[output] = limit
		if output == "max_completion_tokens" {
			delete(body, "max_tokens")
		}
	}
	effort := m.ReasoningEffort
	if m.Reasoning != nil && !*m.Reasoning {
		effort = "none"
	}
	if effort != "" {
		if strings.Contains(path, "responses") {
			reasoning, _ := body["reasoning"].(map[string]any)
			if reasoning == nil {
				reasoning = map[string]any{}
			}
			reasoning["effort"] = effort
			body["reasoning"] = reasoning
		} else if strings.Contains(path, "messages") {
			if effort == "none" {
				body["thinking"] = map[string]any{"type": "disabled"}
				delete(body, "output_config")
			} else {
				body["thinking"] = map[string]any{"type": "adaptive"}
				config, _ := body["output_config"].(map[string]any)
				if config == nil {
					config = map[string]any{}
				}
				config["effort"] = effort
				body["output_config"] = config
			}
		} else {
			body["reasoning_effort"] = effort
		}
	}
	if m.Tier != "" {
		body["service_tier"] = m.Tier
	}
}

func applyRawModelMetadata(body map[string]json.RawMessage, path string, m modelMetadata) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	if m.Tools != nil && !*m.Tools {
		if tools, ok := decoded["tools"].([]any); ok && len(tools) > 0 {
			return errors.New("this model has tool calling disabled")
		}
	}
	applyModelMetadata(decoded, path, m)
	for _, key := range []string{"max_completion_tokens", "max_output_tokens", "max_tokens", "reasoning_effort", "reasoning", "thinking", "output_config", "service_tier"} {
		if value, ok := decoded[key]; ok {
			body[key], err = json.Marshal(value)
			if err != nil {
				return err
			}
		} else {
			delete(body, key)
		}
	}
	return nil
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
	p := provider.Provider{ID: "winone", Name: "WinOne", Key: in.Key}
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
		}
		// An edited endpoint is explicit: do not retain other vendor URLs as
		// invisible fallbacks. An unchanged preset keeps its native protocols.
		if strings.TrimRight(expected, "/") != strings.TrimRight(in.BaseURL, "/") {
			p.Chat, p.Responses, p.Anthropic, p.Decide = "", "", "", ""
		}
		p.ID, p.Key = "winone", in.Key
		p.BaseAPI = in.Protocol
	}
	if in.Model != "" {
		p.Models = []string{in.Model}
		if in.ModelMetadata.ContextWindow > 0 {
			p.Contexts = map[string]int{in.Model: in.ModelMetadata.ContextWindow}
		}
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
	default:
		return p, errors.New("unsupported AI protocol")
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
	} else {
		p, err = configuredProvider(in)
		if err != nil {
			return nil, err
		}
		provider.WinOneProviders = []provider.Provider{p}
		p = provider.All()[0]
	}
	ctx, observed := observeFailures(ctx)
	models, err := p.Fetch(ctx)
	if err != nil {
		if failure := observed.result(); failure != nil && in.Agent == "" {
			return nil, failure
		}
		return nil, errors.New("model discovery failed; models can be entered manually")
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
			result = append(result, modelOption{ID: model.ID, Name: model.Name, Free: model.Free, Metadata: metadata})
			seen[model.ID] = true
		}
	}
	return result, nil
}

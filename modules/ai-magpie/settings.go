package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/pluginrt"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// The public configuration and the upstream runtime share field names. Extra
// WinOne UI properties are intentionally not written into Magpie's profile.
// Accept only the settings that still have an owner in WinOne. Old profile
// switches cannot revive routes, extensions, collection or model downloads.
func applyRuntimeSettings(in request) error {
	provider.WinOneResetSearchModels()
	gateway.WinOneTransportOnly = os.Getenv("WINONE_AI_TRANSPORT_ONLY") == "1"
	var retained struct {
		ClaudePrintMode bool `json:"claudePrintMode"`
	}
	if len(in.MagpieSettings) > 0 && string(in.MagpieSettings) != "null" {
		if err := json.Unmarshal(in.MagpieSettings, &retained); err != nil {
			return errors.New("invalid AI settings")
		}
	}
	s := settings.Settings{NoStats: true, NoUsageStats: true, NoWSLAgents: true, RedactModelOff: true, ClaudePrintMode: retained.ClaudePrintMode, SessionReads: &settings.SessionReads{Asked: true}}
	if !gateway.WinOneTransportOnly {
		s.GatewayConversations = settings.WinOneGatewayRecording()
	}
	settings.WinOneUse(s)
	// Upstream caches model resolution briefly between requests. Host-owned
	// snapshots have no provider-file write to invalidate that cache.
	provider.Changed()
	// No runtime plugin registry is loaded. Key budgets use the fixed core
	// limiter; the account adapters are individually verified on demand.
	pluginrt.WinOneUse(pluginrt.File{Preset: pluginrt.PresetEssentials})
	return nil
}

// The worker must outlive both the drawing and the optional returned image
// download. Magpie still bounds each phase by the user's ImageWait setting.
func workerRequestWait(in request) time.Duration {
	if in.Operation == "gateway" {
		if in.GatewayPath == "/v1/images/generations" || in.GatewayPath == "/v1/images/edits" {
			return 2*settings.Load().ImageWait() + time.Minute
		}
		return 10 * time.Minute
	}
	if in.Operation == "accounts.signin" {
		return 10 * time.Minute
	}
	if in.Operation == "accounts.import" {
		return 3 * time.Minute
	}
	return 90 * time.Second
}

// installModelPreferences feeds the exact upstream preference maps before its
// gateway takes a settings snapshot. SameAs groups pricing/metadata; Wires is
// separately the name sent to the vendor and never rewrites the picker ID.
func installModelPreferences(pid, model string, m modelMetadata) error {
	if _, initialized := settings.WinOneLoad(); !initialized {
		settings.WinOneUse(settings.Load())
	}
	return settings.WinOneEdit(func(s *settings.Settings) error {
		id := pid + "/" + model
		// Upstream maps the listed ID to the vendor ID, not the reverse. Remove
		// the previous alias too so a catalog refresh can clear or rename it.
		for listed, own := range s.ModelIDs {
			if strings.HasPrefix(listed, pid+"/") && own == model {
				clearModelPreferences(s, listed)
				provider.WinOneSetSearchModel(listed, false)
			}
		}
		if m.Alias != "" {
			id = pid + "/" + m.Alias
		}
		// Replace this model's complete override, including cleared fields during
		// catalog hot updates; unrelated models retain their settings.
		clearModelPreferences(s, id)
		if m.Alias != "" {
			if s.ModelIDs == nil {
				s.ModelIDs = map[string]string{}
			}
			s.ModelIDs[id] = model
		}
		// Keep the upstream merge/signing semantics: these apply only to this model,
		// never to a model-list request or to another provider's auxiliary call.
		if len(m.Headers) > 0 {
			if s.ModelHeaders == nil {
				s.ModelHeaders = map[string]map[string]string{}
			}
			s.ModelHeaders[id] = m.Headers
		}
		provider.WinOneSetSearchModel(id, m.Searches)
		if m.Same != "" {
			if s.ModelSameAs == nil {
				s.ModelSameAs = map[string]string{}
			}
			s.ModelSameAs[id] = m.Same
		}
		if m.UpstreamName != "" {
			if s.ModelWires == nil {
				s.ModelWires = map[string]string{}
			}
			s.ModelWires[id] = m.UpstreamName
		}
		if m.API != "" {
			if s.ModelAPIs == nil {
				s.ModelAPIs = map[string]string{}
			}
			s.ModelAPIs[id] = m.API
		}
		if m.Efforts != nil {
			if kept := keptReasoningLevels(m.Levels, m.Efforts); kept != nil {
				if s.ModelEfforts == nil {
					s.ModelEfforts = map[string][]string{}
				}
				s.ModelEfforts[id] = kept
			} else {
				delete(s.ModelEfforts, id)
			}
		}
		// "No reasoning" is upstream's reasonless model (SetModelReasonless):
		// it offers no levels and the gateway sends no reasoning field.
		if m.Reasoning != nil && !*m.Reasoning {
			if s.ModelReasonless == nil {
				s.ModelReasonless = map[string]bool{}
			}
			s.ModelReasonless[id] = true
		} else {
			delete(s.ModelReasonless, id)
		}
		if m.Vision != nil {
			if s.ModelImages == nil {
				s.ModelImages = map[string]bool{}
			}
			s.ModelImages[id] = *m.Vision
		}
		if m.MaxTokens > 0 {
			if s.ModelOutputs == nil {
				s.ModelOutputs = map[string]int{}
			}
			s.ModelOutputs[id] = m.MaxTokens
		}
		// Share the token conversion already used by model discovery and Chat.
		limits := modelLimitSnapshot(provider.Provider{ID: pid}, strings.TrimPrefix(id, pid+"/"), m, cmp.Or(m.ContextWindow, m.ListedContextWindow))
		if threshold := limits.ModelCompacts[id]; threshold > 0 {
			if s.ModelCompacts == nil {
				s.ModelCompacts = map[string]int{}
			}
			s.ModelCompacts[id] = threshold
		}
		if m.Price != nil {
			price := settings.StatedPrice(*m.Price)
			if err := settings.CheckModelPrice(id, price); err != nil {
				return err
			}
			if s.ModelPrices == nil {
				s.ModelPrices = map[string]settings.ModelPrice{}
			}
			s.ModelPrices[id] = price
		}
		return nil
	})
}

// Renaming or clearing an alias retires all of its overrides. Otherwise a
// later model using that name inherits the previous model's request headers.
func clearModelPreferences(s *settings.Settings, id string) {
	delete(s.ModelNames, id)
	delete(s.ModelHeaders, id)
	delete(s.ModelSameAs, id)
	delete(s.ModelWires, id)
	delete(s.ModelAPIs, id)
	delete(s.ModelOutputs, id)
	delete(s.ModelCompacts, id)
	delete(s.ModelPrices, id)
	delete(s.ModelIDs, id)
	delete(s.ModelEfforts, id)
	delete(s.ModelImages, id)
	delete(s.ModelReasonless, id)
}

// keptReasoningLevels is upstream's setModelEfforts/effortsKept without its
// settings write: offered levels, trimmed and lowercased as Magpie reads
// them, in the vendor's order and spelling. nil means none were narrowed
// (all offered), which Magpie stores as no entry. A model with no reported
// levels keeps the declared ones of provider.Levels, in that order, as
// `magpie model efforts` does. A stale level no longer listed is dropped
// rather than refused, so a vendor's renamed ladder never blocks saving.
func keptReasoningLevels(levels, offered []string) []string {
	var keep []string
	for _, level := range offered {
		level = strings.ToLower(strings.TrimSpace(level))
		if level != "" && !slices.Contains(keep, level) {
			keep = append(keep, level)
		}
	}
	if len(keep) == 0 {
		return nil
	}
	if len(levels) == 0 {
		var declared []string
		for _, level := range provider.Levels {
			if slices.Contains(keep, level) {
				declared = append(declared, level)
			}
		}
		return declared
	}
	var vendor, ordered []string
	for _, level := range levels {
		if !slices.ContainsFunc(vendor, func(seen string) bool { return strings.EqualFold(seen, level) }) {
			vendor = append(vendor, level)
		}
	}
	for _, level := range vendor {
		if slices.Contains(keep, strings.ToLower(level)) {
			ordered = append(ordered, level)
		}
	}
	if len(ordered) == 0 || len(ordered) == len(vendor) {
		return nil
	}
	return ordered
}

func testModels(ctx context.Context, in request, output io.Writer) error {
	var p provider.Provider
	if in.Agent != "" {
		a, err := findAccount(ctx, in.Agent)
		if err != nil {
			return err
		}
		p = *a
		p.LimitRestOff = in.LimitRestOff
		if err := applyProviderPreferences(&p, in); err != nil {
			return err
		}
	} else {
		var err error
		p, err = configuredProvider(in)
		if err != nil {
			return err
		}
		provider.WinOneProviders = []provider.Provider{p}
	}
	if !slices.Contains([]string{"", "codex", "claude-code"}, in.TestClient) {
		return errors.New("unsupported test client")
	}
	for id, m := range in.ModelPreferences {
		if err := installModelPreferences(p.ID, id, m); err != nil {
			return err
		}
	}
	ctx = provider.TestAs(ctx, in.TestClient)
	var results []provider.Result
	if len(in.TestModels) > 0 {
		results = p.TestModels(ctx, in.TestModels)
	} else {
		results = p.Test(ctx)
	}
	// The vendor error body can echo a secret. Keep the status, model and
	// timing; return a bounded generic failure instead of upstream text.
	for i := range results {
		if results[i].Error != "" {
			results[i].Error = "Model connectivity test failed"
		}
	}
	return json.NewEncoder(output).Encode(map[string]any{"results": results, "clients": p.TestClients()})
}

// Same-provider models can be described to the upstream bridge without ever
// letting an unknown JSON setting become a provider's endpoint or credential.
type auxiliaryProvider struct {
	provider.Provider
	KeyIDs         map[string]string        `json:"keyIDs,omitempty"`
	AccountKind    string                   `json:"accountKind,omitempty"`
	Metadata       map[string]modelMetadata `json:"metadata,omitempty"`
	ModelNames     map[string]string        `json:"modelNames,omitempty"`
	DecisionSource *request                 `json:"decisionSource,omitempty"`
	NativeSource   *request                 `json:"nativeSource,omitempty"`
	NativeKeys     []provider.KeyAccount    `json:"nativeKeys,omitempty"`
}

func installAuxiliaryProviders(in request) error {
	for _, p := range in.AuxiliaryProviders {
		for key, id := range p.KeyIDs {
			provider.WinOneKeyIdentity(key, id)
		}
		if strings.ContainsAny(p.ID, "/\\\x00") || p.ID == "" {
			return errors.New("invalid auxiliary provider")
		}
		if slices.ContainsFunc(provider.WinOneProviders, func(x provider.Provider) bool { return x.ID == p.ID }) {
			continue
		}
		// Context windows live on Magpie's provider, not its global model
		// settings map. Carry them into the outer router too, so discovery,
		// Ollama show and automatic group selection use the same saved limit.
		for id, metadata := range p.Metadata {
			if metadata.ContextWindow > 0 {
				if p.Contexts == nil {
					p.Contexts = map[string]int{}
				}
				p.Contexts[id] = metadata.ContextWindow
			}
			if metadata.Alias != "" && p.Contexts[id] > 0 {
				p.Contexts[metadata.Alias] = p.Contexts[id]
			}
		}
		provider.WinOneProviders = append(provider.WinOneProviders, p.Provider)
		if p.NativeSource != nil {
			actual, err := configuredProvider(*p.NativeSource)
			if err != nil {
				return err
			}
			actual.Name, actual.Models, actual.Keys = p.Name, p.Models, p.NativeKeys
			// The bridge already mapped every saved slot to its vendor fingerprint.
			actual.KeySpendCaps, actual.KeyOwnBalances = p.KeySpendCaps, p.KeyOwnBalances
			actual.KeyPriceRates = p.KeyPriceRates
			if gateway.WinOneNativeProviders == nil {
				gateway.WinOneNativeProviders = map[string]provider.Provider{}
			}
			gateway.WinOneNativeProviders[p.ID] = actual
		}
		if p.DecisionSource != nil {
			source := *p.DecisionSource
			source.Key = "winone-capability-check"
			actual, err := configuredProvider(source)
			if err != nil {
				return err
			}
			if gateway.WinOneDecisionProviders == nil {
				gateway.WinOneDecisionProviders = map[string]bool{}
			}
			gateway.WinOneDecisionProviders[p.ID] = gateway.WinOneCanDecide(actual)
		}
		// A bridge remains an HTTP provider: attaching an Account would bypass
		// its private transport. Only the onProvider context needs its kind.
		if p.AccountKind != "" {
			if gateway.WinOneMiddlewareAccounts == nil {
				gateway.WinOneMiddlewareAccounts = map[string]string{}
			}
			gateway.WinOneMiddlewareAccounts[p.ID] = p.AccountKind
		}
		for id, m := range p.Metadata {
			if err := installModelPreferences(p.ID, id, m); err != nil {
				return err
			}
		}
		if err := installModelNames(p.ID, p.ModelNames, p.Metadata); err != nil {
			return err
		}
	}
	return nil
}

// Names are display preferences, separate from both public IDs and vendor IDs.
// Replace the provider's names so clearing a name also updates a live catalog.
func installModelNames(pid string, names map[string]string, metadata map[string]modelMetadata) error {
	return settings.WinOneEdit(func(s *settings.Settings) error {
		for id := range s.ModelNames {
			if strings.HasPrefix(id, pid+"/") {
				delete(s.ModelNames, id)
			}
		}
		if s.ModelNames == nil {
			s.ModelNames = map[string]string{}
		}
		for model, name := range names {
			if alias := metadata[model].Alias; alias != "" {
				model = alias
			}
			if name != "" {
				s.ModelNames[pid+"/"+model] = name
			}
		}
		return nil
	})
}

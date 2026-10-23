package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/fx"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/pii"
	"github.com/yetone/magpie/internal/pluginrt"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/redact"
	"github.com/yetone/magpie/internal/settings"
	_ "github.com/yetone/magpie/plugins/all"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// The public configuration and the upstream runtime share field names. Extra
// WinOne UI properties are intentionally not written into Magpie's profile.
func applyRuntimeSettings(in request) error {
	provider.WinOneSearchModels = nil
	gateway.WinOneTransportOnly = os.Getenv("WINONE_AI_TRANSPORT_ONLY") == "1"
	if err := applyBuiltinPlugins(in.MagpieSettings); err != nil {
		return err
	}
	if err := loadBundledSelection(in.MagpieSettings); err != nil {
		return err
	}
	if err := prepareThirdPartyRuntime(in.Operation); err != nil {
		return err
	}
	if in.Operation != "magpie.catalog" && in.Operation != "accounts.catalog" {
		if err := prepareBundledExtensions(context.Background()); err != nil {
			return err
		}
	}
	s := settings.Settings{NoStats: true, NoUsageStats: true}
	if len(in.MagpieSettings) > 0 && string(in.MagpieSettings) != "null" {
		if err := json.Unmarshal(in.MagpieSettings, &s); err != nil {
			return errors.New("invalid Magpie settings")
		}
	}
	// WinOne language tags use regions; Magpie's saved settings validate its
	// own locale keys. Normalize before any route preference can be saved.
	switch s.Lang {
	case "zh-CN", "zh-Hans":
		s.Lang = "zh"
	case "zh-Hant", "zh-HK":
		s.Lang = "zh-TW"
	default:
		if strings.HasPrefix(s.Lang, "en-") {
			s.Lang = "en"
		}
	}
	rules, err := redact.CheckRules(s.RedactRules)
	if err != nil {
		return err
	}
	s.RedactRules = rules
	// WinOne owns a private profile; upstream discovery must not reach real
	// WSL agent accounts, even when an imported preference requests discovery.
	s.NoWSLAgents = true
	var extra struct {
		SearchAPIs []provider.SearchAPI `json:"searchAPIs"`
	}
	if len(in.MagpieSettings) > 0 {
		if err = json.Unmarshal(in.MagpieSettings, &extra); err != nil {
			return err
		}
	}
	provider.WinOneSearchAPIs = append([]provider.SearchAPI{}, extra.SearchAPIs...)
	address, key := os.Getenv("WINONE_AI_PRIVACY_URL"), os.Getenv("WINONE_AI_PRIVACY_KEY")
	if err := pii.WinOneUseRemote(address, key); err != nil {
		return err
	}
	// A short worker gets this machine's opt-in from the Host, not from an
	// account-specific preference file. The shared router keeps native Find.
	if address != "" {
		s.RedactModel = true
	}
	settings.WinOneUse(s)
	// These protocol features now register through Magpie's plugin runtime.
	// Compose their actual upstream implementations in WinOne's private
	// profile; never bypass the gateway's fail-closed privacy check.
	_, err = pluginrt.Refresh()
	return err
}

// The worker must outlive both the drawing and the optional returned image
// download. Magpie still bounds each phase by the user's ImageWait setting.
func workerRequestWait(in request) time.Duration {
	if in.Operation == "gateway" || in.Operation == "magpie.gateway" {
		if in.GatewayPath == "/v1/images/generations" || in.GatewayPath == "/v1/images/edits" {
			return 2*settings.Load().ImageWait() + time.Minute
		}
		return 10 * time.Minute
	}
	if in.Operation == "accounts.signin" {
		return 10 * time.Minute
	}
	return 90 * time.Second
}

// installModelPreferences feeds the exact upstream preference maps before its
// gateway takes a settings snapshot. SameAs groups pricing/metadata; Wires is
// separately the name sent to the vendor and never rewrites the picker ID.
func installModelPreferences(pid, model string, m modelMetadata) error {
	s := settings.Load()
	id := pid + "/" + model
	if m.Searches {
		if provider.WinOneSearchModels == nil { provider.WinOneSearchModels = map[string]bool{} }
		provider.WinOneSearchModels[id] = true
	}
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
		if s.ModelEfforts == nil {
			s.ModelEfforts = map[string][]string{}
		}
		s.ModelEfforts[id] = slices.Clone(m.Efforts)
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
	settings.WinOneUse(s)
	return nil
}

type settingModel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	ProviderName string `json:"providerName"`
	Icon         string `json:"icon,omitempty"`
}

func settingRef(p provider.Provider, m catalog.Model) settingModel {
	name := m.Name
	if name == "" {
		name = m.ID
	}
	return settingModel{ID: p.ID + "/" + m.ID, Name: name, Provider: p.ID, ProviderName: p.Name, Icon: p.Icon}
}

func magpieCatalog(ctx context.Context, in request, output io.Writer) error {
	// A catalog request can include one isolated account or the host's private
	// model bridge. Neither path enumerates a real external application home.
	provider.WinOneProviders = []provider.Provider{}
	if in.Agent != "" {
		if _, err := findAccount(ctx, in.Agent); err != nil {
			return err
		}
	}
	if err := installAuxiliaryProviders(in); err != nil {
		return err
	}
	searchVendors := []map[string]any{}
	for _, v := range provider.SearchVendors {
		searchVendors = append(searchVendors, map[string]any{"id": v.ID, "name": v.Name, "keysURL": v.KeysURL, "needURL": v.Base == "", "keyOptional": v.KeyOptional})
	}
	categories := []map[string]any{}
	for _, c := range redact.Categories {
		categories = append(categories, map[string]any{"id": c.ID, "name": c.ID, "on": c.On})
	}
	visionModels := []settingModel{}
	titleModels := []settingModel{}
	imageModels := []settingModel{}
	for _, e := range provider.Served() {
		if e.Provider.Ready() && !e.Provider.DecideOnly() {
			titleModels = append(titleModels, settingModel{ID: e.ID, Name: e.Name, Provider: e.Provider.ID, ProviderName: e.Provider.Name, Icon: e.Provider.Icon})
		}
		if e.Images && (e.ImageInput == nil || *e.ImageInput) && e.Provider.Ready() {
			visionModels = append(visionModels, settingModel{ID: e.ID, Name: e.Name, Provider: e.Provider.ID, ProviderName: e.Provider.Name, Icon: e.Provider.Icon})
		}
	}
	for _, p := range provider.All() {
		for _, m := range gateway.Drawers(p) {
			imageModels = append(imageModels, settingRef(p, m))
		}
	}
	choices := []map[string]any{}
	for _, c := range gateway.Searchers() {
		models := []settingModel{}
		for _, m := range c.Models {
			models = append(models, settingRef(c.Provider, m))
		}
		choices = append(choices, map[string]any{"id": c.Provider.ID, "name": c.Provider.Name, "icon": c.Provider.Icon, "small": c.Small, "models": models, "service": c.Service, "own": c.Own})
	}
	relays := []string{}
	for _, p := range gateway.RelaysSaidToSearch() {
		relays = append(relays, p.Name)
	}
	leftOut := []string{}
	for _, p := range provider.All() {
		if p.On() && !slices.ContainsFunc(choices, func(c map[string]any) bool { return c["id"] == p.ID }) && !slices.Contains(relays, p.Name) {
			leftOut = append(leftOut, p.Name)
		}
	}
	users := []string{}
	if in.Agent == "codex" {
		for _, l := range provider.Logins("codex") {
			users = append(users, l.User)
		}
	}
	checkins := provider.PluginCheckins()
	fxctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	rate := fx.Get(fxctx)
	cancel()
	plugins, err := builtinPluginCatalog()
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"builtinPlugins": plugins, "bundledPlugins": accountPlugins(), "searchVendors": searchVendors, "redactionCategories": categories, "redactCategories": categories, "visionModels": visionModels, "titleModels": titleModels, "visionAuto": gateway.AutoVision(), "visionMissing": gateway.VisionMissing(), "imageGenModels": imageModels, "imageGenAuto": gateway.AutoDrawer(), "imageGenMissing": gateway.DrawerMissing(), "searchChoices": choices, "searchAuto": gateway.AutoSearcher(), "searchProvider": gateway.Searcher(), "searchUnused": gateway.SearcherUnused(), "searchRelays": relays, "searchLeftOut": leftOut, "pluginCheckins": checkins, "checkinPlugins": checkins, "codexUsers": users, "noAccountsEnv": false, "fx": map[string]any{"rate": rate.CNYPerUSD, "at": rate.At, "stale": rate.Stale()}})
}

func testModels(ctx context.Context, in request, output io.Writer) error {
	var p provider.Provider
	if in.Agent != "" {
		a, err := findAccount(ctx, in.Agent)
		if err != nil {
			return err
		}
		p = *a
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

// maintenance is called only while the user enabled a quota action. It uses
// persisted upstream due-state, so a restarted Host cannot duplicate check-in
// credits or scheduled warm-ups. A disabled account never reaches this path.
func maintainMagpie(ctx context.Context, in request, output io.Writer) error {
	if in.Agent == "" {
		return errors.New("an account is required for quota maintenance")
	}
	if _, err := findAccount(ctx, in.Agent); err != nil {
		return err
	}
	s := settings.Load()
	checkins := []provider.WorkBuddyCheckin{}
	if provider.PluginCheckinOn(s, in.Agent) {
		checkins = append(checkins, provider.WinOneCheckInPluginsDue(ctx, provider.PluginID(in.Agent))...)
	}
	warm := provider.WinOneWarmNow(ctx, gateway.WinOneWarmClaude)
	if len(s.CodexAutoReset) > 0 {
		provider.SpendExpiringCodexResets(ctx)
	}
	agent := in.Agent
	if _, ok := pluginForAgent(in.Agent); ok {
		agent = provider.PluginID(agent)
	}
	readings := provider.LoginUsage(ctx, agent)
	quotas := []provider.SubscriptionQuota{}
	for user, q := range readings {
		q.User = user
		quotas = append(quotas, q)
	}
	alerts := provider.WinOneQuotaAlerts(quotas)
	issues := []string{}
	for _, w := range warm {
		if w.Err != "" {
			issues = append(issues, "Account warm-up failed")
		}
	}
	for _, c := range checkins {
		if c.Outcome == provider.CheckinFailed {
			issues = append(issues, "Daily check-in failed")
		}
	}
	return json.NewEncoder(output).Encode(map[string]any{"quotas": quotas, "alerts": alerts, "checkins": checkins, "errors": issues, "trayCards": trayCards(quotas, s.TrayUsages, in.ProviderID, time.Now())})
}

// Adapted from upstream gui/trayusage.go: resolve an explicit card or the
// account in use, then keep only quota windows that stop the whole account.
func trayCards(cards []provider.SubscriptionQuota, ids []string, hostID string, now time.Time) []provider.SubscriptionQuota {
	result := []provider.SubscriptionQuota{}
	for _, id := range ids {
		pid, user, _ := strings.Cut(id, "|")
		mine := []provider.SubscriptionQuota{}
		for _, q := range cards {
			if q.Provider == pid || pid == hostID {
				mine = append(mine, q)
			}
		}
		if len(mine) == 0 {
			continue
		}
		index := -1
		if user == "*" {
			index = 0
			last := -1
			for i, q := range mine {
				if q.LastServedAt != nil && (last < 0 || q.LastServedAt.After(*mine[last].LastServedAt)) {
					last = i
				}
			}
			in := -1
			if who := provider.InUseLogin(mine[0].Provider); who != "" {
				in = slices.IndexFunc(mine, func(q provider.SubscriptionQuota) bool { return strings.EqualFold(q.User, who) })
			}
			switch {
			case last >= 0 && now.Sub(*mine[last].LastServedAt) < time.Hour:
				index = last
			case in >= 0:
				index = in
			case last >= 0:
				index = last
			}
		} else {
			index = slices.IndexFunc(mine, func(q provider.SubscriptionQuota) bool { return q.User == user })
		}
		if index < 0 {
			continue
		}
		q := mine[index]
		windows := []provider.QuotaWindow{}
		for _, w := range q.Windows {
			if !w.Aside && w.Model == "" {
				windows = append(windows, w)
			}
		}
		slices.SortStableFunc(windows, func(a, b provider.QuotaWindow) int {
			if a.Span == 0 || b.Span == 0 {
				return 0
			}
			return cmp.Compare(a.Span, b.Span)
		})
		q.Windows = windows
		if q.Alias != "" {
			q.Name += " · " + q.Alias
		} else if user == "*" && q.User != "" {
			q.Name += " · " + q.User
		}
		result = append(result, q)
	}
	return result
}

// Same-provider models can be described to the upstream bridge without ever
// letting an unknown JSON setting become a provider's endpoint or credential.
type auxiliaryProvider struct {
	provider.Provider
	AccountKind string                   `json:"accountKind,omitempty"`
	Metadata    map[string]modelMetadata `json:"metadata,omitempty"`
}

func installAuxiliaryProviders(in request) error {
	for _, p := range in.AuxiliaryProviders {
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
		}
		provider.WinOneProviders = append(provider.WinOneProviders, p.Provider)
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
	}
	return nil
}

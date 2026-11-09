package aiservices

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/widgets"
)

// Native routing is a presentation of Magpie's groups/trace APIs, not a
// replacement router. Its form retains unknown upstream fields on roundtrip.
type routeGroup struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	Members    []string                     `json:"members"`
	Match      []string                     `json:"match,omitempty"`
	Matched    []string                     `json:"matched,omitempty"`
	Placed     []string                     `json:"placed,omitempty"`
	Routing    string                       `json:"routing,omitempty"`
	Affinity   string                       `json:"affinity,omitempty"`
	Sink       bool                         `json:"sink,omitempty"`
	NoSink     bool                         `json:"noSink,omitempty"`
	FirstToken int                          `json:"firstToken,omitempty"`
	Patience   int                          `json:"patience,omitempty"`
	Off        []string                     `json:"off,omitempty"`
	Pick       string                       `json:"pick,omitempty"`
	Rules      []core.Preserved[routeRule]  `json:"rules,omitempty"`
	Classifier string                       `json:"classifier,omitempty"`
	IntentSure *float64                     `json:"intentSure,omitempty"`
	Effort     string                       `json:"effort,omitempty"`
	Context    int                          `json:"context,omitempty"`
	Levels     []string                     `json:"levels,omitempty"`
	Fast       []string                     `json:"fast,omitempty"`
	Weights    map[string]int               `json:"weights,omitempty"`
	EffortMaps map[string]map[string]string `json:"effortMaps,omitempty"`
	Family     string                       `json:"family,omitempty"`
	Unlists    bool                         `json:"unlists,omitempty"`
	Accounts   []string                     `json:"accounts,omitempty"`
	Auto       bool                         `json:"auto,omitempty"`
	Hidden     bool                         `json:"hidden,omitempty"`
	Seen       []string                     `json:"seen,omitempty"`
	Disabled   bool                         `json:"disabled,omitempty"`
	Ready      bool                         `json:"ready"`
	Ref        string                       `json:"ref"`
	Patterns   []patternMatch               `json:"patterns"`
	MemberInfo []routeModel                 `json:"memberInfo"`
	Offers     []string                     `json:"offers"`
	Shared     []string                     `json:"shared"`
}
type routeRule struct {
	Use           string     `json:"use"`
	More          []string   `json:"more,omitempty"`
	UseEffort     string     `json:"useEffort,omitempty"`
	Tokens        int        `json:"tokens,omitempty"`
	Images        bool       `json:"images,omitempty"`
	Effort        string     `json:"effort,omitempty"`
	Agents        []string   `json:"agents,omitempty"`
	Intent        string     `json:"intent,omitempty"`
	Compact       bool       `json:"compact,omitempty"`
	Time          *routeTime `json:"time,omitempty"`
	Folders       []string   `json:"folders,omitempty"`
	ExceptFolders []string   `json:"exceptFolders,omitempty"`
	Pause         bool       `json:"pause,omitempty"`
}
type routeTime struct {
	Days []string `json:"days,omitempty"`
	From string   `json:"from"`
	To   string   `json:"to"`
	Zone string   `json:"zone,omitempty"`
}
type routeModel struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	ProviderName string   `json:"providerName"`
	Efforts      []string `json:"efforts"`
	CanFast      bool     `json:"canFast"`
	Context      int      `json:"context"`
	Images       bool     `json:"images"`
	Free         bool     `json:"free"`
	Zero         bool     `json:"zero"`
}
type routePool struct {
	Provider string   `json:"provider"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Who      []string `json:"who"`
	Routing  string   `json:"routing"`
	Affinity string   `json:"affinity"`
	Sink     bool     `json:"sink"`
	Protocol string   `json:"protocol"`
}

type routeCapabilities struct {
	Builtins   []builtinPlugin `json:"builtins"`
	JSRoutings []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Plugin string `json:"plugin"`
	} `json:"jsRoutings"`
}

type routeGroups struct {
	Groups    []core.Preserved[routeGroup] `json:"groups"`
	Models    []routeModel                 `json:"models"`
	Deciders  []routeModel                 `json:"deciders"`
	Drawers   []routeModel                 `json:"drawers"`
	Pools     []routePool                  `json:"pools"`
	Found     bool                         `json:"found"`
	Skip      []string                     `json:"skip"`
	Skippable []named                      `json:"skippable"`
	Templates []struct {
		Kind  string                     `json:"kind"`
		Group core.Preserved[routeGroup] `json:"group"`
		Why   string                     `json:"why"`
		Need  string                     `json:"need"`
		Of    string                     `json:"of"`
	} `json:"templates"`
	Accounts map[string][]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"accounts"`
	Clock    json.RawMessage `json:"clock"`
	Shared   string          `json:"shared"`
	Imported *routeImport    `json:"imported"`
}

type routeImport struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Missing  []string `json:"missing"`
	Instead  []string `json:"instead"`
	Rules    int      `json:"rules"`
	Replaced bool     `json:"replaced"`
}

type routingPanel struct {
	p                                *page
	api, capabilitiesAPI, patternAPI nativeAPI
	previewPatterns                  []patternMatch
	previewFor                       []string
	capabilities                     *routeCapabilities
	state                            *routeGroups
	source                           string
	tab                              int
	editorTab                        int
	search                           string
	selected                         map[string]bool
	edit, saved                      *core.Preserved[routeGroup]
	fields                           fields
	memberSearch                     string
	newMember                        string
	dialog, text, name               string
	replaceImport                    bool
	newGroup, showHidden             bool
	imported                         *routeImport
	trace                            nativeTrace
}

func newRoutingPanel(p *page) *routingPanel {
	r := &routingPanel{p: p, tab: 1, selected: map[string]bool{}, fields: fields{}}
	if p != nil {
		r.trace.loadViewPrefs(p.env.Directory)
	}
	return r
}
func (r *routingPanel) Close() { r.Leave() }
func (r *routingPanel) Leave() {
	r.api.close()
	r.capabilitiesAPI.close()
	r.patternAPI.close()
	r.trace.api.close()
	r.trace.detailAPI.close()
	r.trace.titleAPI.close()
	r.trace.routeAPI.close()
	r.trace.daysAPI.close()
	r.trace.stopReplay()
	if r.trace.quotas != nil {
		r.trace.quotas.Close()
	}
	r.source = ""
	r.p.env.Activity.Report("ai-routing", false, false, nil)
}
func (r *routingPanel) dirty() bool { return r.edit != nil && (r.newGroup || !equal(r.edit, r.saved)) }
func (r *routingPanel) discard()    { r.edit, r.saved = nil, nil; r.newGroup = false; r.fields = fields{} }
func (r *routingPanel) load() {
	if r.capabilities == nil && !r.capabilitiesAPI.busy && r.capabilitiesAPI.err == "" {
		r.capabilitiesAPI.request(r.p, "GET", "/api/settings", nil, func(raw json.RawMessage) {
			var value routeCapabilities
			if nativeDecode(&r.capabilitiesAPI, raw, &value) {
				r.capabilities = &value
			}
		})
	}
	r.api.request(r.p, "GET", "/api/groups", nil, func(raw json.RawMessage) {
		var value routeGroups
		if decodeRouteGroups(&r.api, raw, &value) {
			r.state = &value
		}
	})
}
func (r *routingPanel) act(action string, body any, done func()) {
	r.api.request(r.p, "POST", "/api/groups/"+action, body, func(raw json.RawMessage) {
		var value routeGroups
		if !decodeRouteGroups(&r.api, raw, &value) {
			return
		}
		r.state = &value
		if value.Imported != nil {
			r.imported = value.Imported
		}
		if done != nil {
			done()
		}
	})
}
func (r *routingPanel) begin(group core.Preserved[routeGroup]) {
	r.beginEdit(group, false)
}

// Templates have an ID but are still new. Never send that ID as a rename source.
func (r *routingPanel) beginNew(group core.Preserved[routeGroup]) {
	r.beginEdit(group, true)
}
func (r *routingPanel) beginEdit(group core.Preserved[routeGroup], fresh bool) {
	change := func() {
		data, _ := json.Marshal(group)
		var a, b core.Preserved[routeGroup]
		_ = json.Unmarshal(data, &a)
		_ = json.Unmarshal(data, &b)
		r.patternAPI.close()
		r.previewPatterns, r.previewFor = nil, nil
		r.edit, r.saved = &a, &b
		r.tab = 0
		r.newGroup = fresh
		r.editorTab = 0
		r.fields = fields{}
	}
	if r.dirty() && r.p.env.RequestChange != nil {
		r.p.env.RequestChange(change)
	} else {
		change()
	}
}
func (r *routingPanel) saveGroup() {
	if r.edit == nil {
		return
	}
	data, _ := json.Marshal(r.edit)
	var body map[string]json.RawMessage
	_ = json.Unmarshal(data, &body)
	if !r.newGroup {
		body["from"], _ = json.Marshal(r.saved.Value.ID)
	}
	r.act("save", body, r.discard)
}
func (r *routingPanel) View(c *ui.Context) {
	p := r.p
	p.env.Activity.Report("ai-routing", r.dirty(), r.api.busy, r.discard)
	if replacement := p.extensions.replacement("routing"); replacement != nil {
		p.extensions.Page(c, *replacement, nil, p.locked())
		return
	}
	if p.extensions.state != nil && r.source != p.extensions.state.URL {
		r.api.close()
		r.capabilitiesAPI.close()
		r.capabilitiesAPI.err = ""
		r.patternAPI.close()
		r.patternAPI.err = ""
		r.previewPatterns, r.previewFor = nil, nil
		r.capabilities = nil
		r.trace.api.close()
		r.trace.detailAPI.close()
		r.trace.titleAPI.close()
		r.trace.routeAPI.close()
		r.trace.daysAPI.close()
		r.trace.stopReplay()
		r.trace.daysLoaded = false
		r.trace.daysAPI.err = ""
		r.trace.days, r.trace.routes = nil, nil
		r.trace.tallies, r.trace.titles = nil, nil
		r.trace.nextTitles = time.Time{}
		r.trace.seq = 0
		if r.trace.quotas != nil {
			r.trace.quotas.Close()
			r.trace.quotas = nil
		}
		r.trace.loaded, r.trace.selected = false, nil
		r.source = p.extensions.state.URL
		r.load()
	}
	if r.edit != nil {
		p.env.Rail.Set(
			core.Action{Label: nativeT("saveGroup"), Disabled: !r.dirty() || r.api.busy || p.locked(), Busy: r.api.busy, Run: r.saveGroup},
			core.Action{Disabled: !r.dirty() || r.api.busy, Run: r.discard},
		)
	}
	if p.draft != nil {
		kit.Toggle(c, &p.draft.Value.RoutingEnabled, i18n.T("routing.enabled"), i18n.T("routing.hint")).Disabled(p.locked() || r.api.busy)
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		label := upstreamT("Routing groups")
		target := 0
		if r.tab == 0 {
			label = upstreamT("Routing")
			target = 1
		}
		if kit.GhostButton(c, label, nil, false).Clicked() {
			change := func() { r.tab = target }
			if p.env.RequestChange != nil {
				p.env.RequestChange(change)
			} else {
				change()
			}
		}
		ui.Spacer(c).Grow(1)
		if p.draft != nil {
			kit.Toggle(c, &p.draft.Value.settings().HideAccounts, upstreamT("Hide accounts"), "").Disabled(p.locked())
		}
	})
	r.api.notice(c, r.load)
	r.capabilitiesAPI.notice(c, func() { r.capabilitiesAPI.err = ""; r.capabilities = nil; r.load() })
	if r.capabilities != nil {
		for _, feature := range r.capabilities.Builtins {
			if feature.ID == "groups" && !feature.On {
				kit.Notice(c, kit.Info, "", nativeT("groupsDisabled"), func() {
					if kit.Button(c, nativeT("enableGroups"), nil, false).Disabled(p.locked()).Clicked() {
						p.enableRoutingGroups(func(err error) {
							if err != nil {
								r.api.err = err.Error()
							}
						})
					}
				})
			}
		}
	}
	if r.tab == 1 {
		r.trace.View(c, p)
		return
	}
	// Hidden request views must not keep polling or advance a replay's clock.
	if r.trace.api.busy {
		r.trace.api.close()
	}
	r.trace.daysAPI.close()
	r.trace.titleAPI.close()
	if r.trace.quotas != nil {
		r.trace.quotas.Close()
	}
	if r.trace.replay != nil {
		r.trace.replay.last = time.Time{}
	}
	if r.state == nil {
		if r.api.busy || p.extensions.state == nil {
			kit.Status(c, i18n.T("capture.loading"))
		}
		return
	}
	if r.tab == 2 {
		r.poolsView(c)
	} else {
		r.groupsView(c)
	}
	r.dialogView(c)
}

// pageURL carries only public identity to an author's own plugin surface.
func pageURL(base string, r *pageRequest) string {
	u, err := url.Parse(base)
	if err != nil || r == nil {
		return base
	}
	q := u.Query()
	q.Set("surface", "1")
	q.Set("page", r.key)
	q.Set("request", r.nonce)
	if r.action != "" && r.menu != "" && r.item != nil {
		q.Set("action", r.action)
		q.Set("menu", r.menu)
		q.Set("item-id", r.item.ID)
		q.Set("item-name", r.item.Name)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *page) openPluginProvider(a agent, signIn bool) error {
	if p.draft == nil {
		return errors.New(i18n.T("capture.loading"))
	}
	var existing *provider
	for i := range p.draft.Value.Providers {
		pr := &p.draft.Value.Providers[i].Value
		if pr.AuthMode == "account" && pr.Agent == a.ID && pr.AgentBackend == "plugin" {
			existing = pr
			break
		}
	}
	if existing == nil && len(p.draft.Value.Providers) >= maxProviders {
		return errors.New(i18n.T("aiPlugins.providerLimit"))
	}
	p.switchTab(tabAccounts)
	p.providerTab = providerConnection
	id := ""
	if existing != nil {
		id = existing.ID
	} else {
		id = p.add(&a)
	}
	p.selected = id
	if signIn {
		p.extensions.signIn = &signInRequest{provider: id, nonce: nonce()}
	}
	return nil
}

var maskKinds = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func nativeSelect(c *ui.Context, label string, current *string, choices []kit.SelectOption, disabled bool) bool {
	changed := false
	ui.Field(c, label, func() {
		var trigger ui.Element
		changed, trigger = kit.Select(c, current, choices)
		trigger.FillWidth().MinWidth(0).Label(label).Disabled(disabled)
	})
	return changed
}

func nativeText(c *ui.Context, label string, value *string, disabled bool) {
	ui.Field(c, label, func() { ui.TextInput(c, value).FillWidth().Label(label).Disabled(disabled) })
}
func nativeInt(c *ui.Context, label string, value *int, low, high int, disabled bool) bool {
	changed := false
	n := float64(*value)
	numberRow(c, label, &n, float64(low), float64(high), true, "", disabled, func(v *float64) { *value = intNumber(v); changed = true })
	return changed
}
func intNumber(v *float64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}
func nativeList(c *ui.Context, f fields, key, label string, values *[]string, disabled bool) {
	s := f.get(key)
	if value, changed := s.sync(strings.Join(*values, "\n")); changed {
		var out []string
		for _, line := range strings.Split(value, "\n") {
			if line = strings.TrimSpace(line); line != "" && !slices.Contains(out, line) {
				out = append(out, line)
			}
		}
		*values = out
	}
	ui.Field(c, label, func() { ui.TextArea(c, &s.text).FillWidth().MinHeight(64).Label(label).Disabled(disabled) })
}

// nativeModelField is only a data adapter to the same model menu Chat uses.
func nativeModelField(c *ui.Context, p *page, label string, current *string, models []routeModel, choices []widgets.ModelChoice, locked bool) {
	if *current != "" && !slices.ContainsFunc(models, func(m routeModel) bool { return m.ID == *current }) && !slices.ContainsFunc(choices, func(v widgets.ModelChoice) bool { return v.Value == *current }) {
		choices = append(slices.Clone(choices), widgets.ModelChoice{Value: *current, Label: p.routing.memberLabel(*current)})
	}
	var groups []widgets.ModelGroup
	indices := map[string]int{}
	for _, model := range models {
		provider := model.Provider
		if provider == "" {
			provider, _, _ = strings.Cut(model.ID, "/")
		}
		index, ok := indices[provider]
		if !ok {
			index = len(groups)
			indices[provider] = index
			name := model.ProviderName
			if name == "" {
				name = p.routing.providerLabel(provider)
			}
			groups = append(groups, widgets.ModelGroup{ID: provider, Name: name})
		}
		name := model.Name
		if name == "" {
			name = p.routing.memberLabel(model.ID)
		}
		groups[index].Models = append(groups[index].Models, widgets.ModelEntry{Value: model.ID, ID: model.ID, Name: name, Label: name})
	}
	ui.Field(c, label, func() {
		if v, picked := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: p.env, Groups: groups, Choices: choices, Selected: *current, Label: label}, Disabled: locked}); picked {
			*current = v
		}
	})
}

func toggleString(values []string, value string, on bool) []string {
	out := slices.Clone(values)
	if on && !slices.Contains(out, value) {
		return append(out, value)
	}
	if !on {
		return slices.DeleteFunc(out, func(v string) bool { return v == value })
	}
	return out
}

// Upstream strategy words are sent unchanged; plugin-defined strategies remain
// editable as their own identifiers instead of being dropped by a whitelist.
func routingChoices() []kit.SelectOption {
	var out []kit.SelectOption
	for _, id := range []string{"", "order", "rotate", "usage", "pace", "weight", "free", "fast", "cheap", "drain", "expiring", "manual"} {
		key := id
		if key == "" {
			key = "smart"
		}
		out = append(out, kit.SelectOption{Value: id, Label: i18n.T("aiNative.strategy." + key)})
	}
	return out
}

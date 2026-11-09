package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Use the runtime catalog (including in-use dependencies), not unsaved switches,
// to match upstream's feature visibility. Until it arrives, keep saved choices.
func (p *page) builtinEnabled(id string) bool {
	if p.advanced != nil && p.advanced.catalog != nil {
		for _, plugin := range p.advanced.catalog.BuiltinPlugins {
			if plugin.ID == id {
				return plugin.On || plugin.InUse > 0
			}
		}
	}
	if p.saved != nil {
		s := p.saved.Value.settings()
		if on := s.BuiltinPlugins[id].Value.Enabled; on != nil {
			return *on
		}
		if s.BuiltinPreset == "essentials" {
			return !slices.Contains([]string{"sessions", "library", "groups", "warmups", "otel"}, id)
		}
	}
	return true
}

func (p *page) destinationEnabled(tab int) bool {
	id := map[int]string{tabSessions: "sessions", tabLibrary: "library", tabRouting: "routing", tabGateway: "gateway", tabUsage: "usage", tabMasking: "redact"}[tab]
	return id == "" || p.builtinEnabled(id)
}

// Keep every upstream destination, with the user's requested top navigation.
// Routing and its groups share a service but remain separate destinations.
func (p *page) upstreamNavigation(c *ui.Context) {
	type destination struct {
		label string
		tab   int
		route int
	}
	entries := []destination{
		{homeT("home"), tabHome, -1},
		{i18n.T("ai.customProviders"), tabCustom, -1}, {i18n.T("ai.addProvidersTab"), tabAccounts, -1},
		{upstreamT("Sessions"), tabSessions, -1}, {upstreamT("Library"), tabLibrary, -1},
		{upstreamT("Routing"), tabRouting, 1}, {upstreamT("Routing groups"), tabRouting, 0},
		{upstreamT("Gateway"), tabGateway, -1}, {upstreamT("Usage"), tabUsage, -1}, {upstreamT("Masking"), tabMasking, -1},
		{upstreamT("Plugins"), tabPlugins, -1}, {homeT("preferences"), tabDefaults, -1}, {i18n.T("ai.advanced"), tabAdvanced, -1},
	}
	entries = slices.DeleteFunc(entries, func(e destination) bool { return !p.destinationEnabled(e.tab) })
	if !p.destinationEnabled(p.tab) {
		p.switchTab(tabHome)
	}
	current := 0
	tabs := make([]kit.Tab, len(entries))
	for i, e := range entries {
		tabs[i] = kit.Tab{Label: e.label}
		if p.tab == e.tab && (e.route < 0 || p.routing.tab == e.route) {
			current = i
		}
	}
	ui.Column(c).FillWidth().Padding(12, 20, 0, 20).Gap(10).Disabled(p.navigationLocked()).Children(func() {
		kit.Tabs(c, &current, tabs, func(apply func()) {
			change := func() {
				apply()
				e := entries[current]
				if p.tab != e.tab {
					p.switchTab(e.tab)
				}
				if e.route >= 0 {
					p.routing.tab = e.route
				}
				p.scroll.ScrollTo(0, 0, ui.Animation{})
			}
			if p.env.RequestChange != nil {
				p.env.RequestChange(change)
			} else {
				change()
			}
		})
		if p.tab == tabPlugins {
			kit.SubTabs(c, &p.plugins.tab, []string{upstreamT("Built-in"), upstreamT("Discover"), upstreamT("Installed")}, p.env.RequestChange)
		}
	})
}

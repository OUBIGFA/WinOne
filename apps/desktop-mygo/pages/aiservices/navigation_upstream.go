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
	// These optional product surfaces were explicitly removed. Their shared
	// upstream libraries may still be used by plugin tools, never by navigation.
	if id == "sessions" || id == "library" {
		return false
	}
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
	id := map[int]string{tabRouting: "routing", tabGateway: "gateway", tabUsage: "usage", tabMasking: "redact"}[tab]
	return id == "" || p.builtinEnabled(id)
}

// Four everyday tasks stay first. Secondary capabilities share one destination
// so widening the feature catalog never widens the primary navigation.
func (p *page) upstreamNavigation(c *ui.Context) {
	type destination struct {
		label string
		tab   int
	}
	entries := []destination{
		{homeT("home"), tabHome},
		{i18n.T("ai.customProviders"), tabCustom}, {i18n.T("ai.addProvidersTab"), tabAccounts},
		{homeT("preferences"), tabDefaults}, {homeT("more"), tabMore},
	}
	if !p.destinationEnabled(p.tab) {
		p.switchTab(tabHome)
	}
	current := len(entries) - 1
	tabs := make([]kit.Tab, len(entries))
	for i, e := range entries {
		tabs[i] = kit.Tab{Label: e.label}
		if p.tab == e.tab {
			current = i
		}
	}
	ui.Column(c).FillWidth().Gap(kit.HeadingGap).Disabled(p.navigationLocked()).Children(func() {
		kit.Tabs(c, &current, tabs, func(apply func()) {
			change := func() {
				apply()
				e := entries[current]
				if p.tab != e.tab {
					p.switchTab(e.tab)
				}
			}
			if p.env.RequestChange != nil {
				p.env.RequestChange(change)
			} else {
				change()
			}
		})
		if current == len(entries)-1 && p.tab != tabMore {
			p.moreHeading(c)
		}
		if p.tab == tabPlugins {
			kit.SubTabs(c, &p.plugins.tab, []string{upstreamT("Built-in"), upstreamT("Discover"), upstreamT("Installed")}, p.env.RequestChange)
		}
	})
}

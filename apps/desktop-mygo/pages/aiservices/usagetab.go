//go:build usage_ready

package aiservices

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/pages/aiservices/usage"
)

// usageTab is the usage statistics of package usage.
type usageTab = usage.Usage

// usageView is the usage statistics, or the plugin page that replaces
// them, with the panels plugins add below.
func (p *page) usageView(c *ui.Context) {
	if page := p.extensions.replacement("usage"); page != nil {
		p.extensions.Page(c, *page, nil, p.locked())
		return
	}
	if p.usage == nil {
		p.usage = usage.New(p.env)
	}
	o := usage.Options{Active: true, Disabled: p.locked()}
	if p.saved != nil {
		o.Providers = usageProviders(p.saved.Value.Providers)
		o.Revision = int64(p.saved.Value.Revision)
	}
	if p.draft != nil {
		s := p.draft.Value.magpie()
		o.Preferences = &usage.Preferences{HideAccounts: s.HideAccounts, QuotaLeft: s.QuotaLeft, Aliases: s.UsageAliases}
		o.Change = func(prefs usage.Preferences) {
			s := p.draft.Value.settings()
			s.HideAccounts, s.QuotaLeft, s.UsageAliases = prefs.HideAccounts, prefs.QuotaLeft, prefs.Aliases
		}
	}
	p.usage.View(c, o)
	p.extensions.Slot(c, "usage", nil, p.locked())
}

// usageProviders converts the saved providers for the statistics.
func usageProviders(docs []providerDoc) []usage.Provider {
	out := make([]usage.Provider, len(docs))
	for i := range docs {
		pr := &docs[i].Value
		out[i] = usage.Provider{AIProvider: pr.picker(), AuthMode: pr.AuthMode, Agent: pr.Agent, AgentBackend: pr.AgentBackend,
			HasKey: pr.HasKey, BaseURL: pr.BaseURL}
		if pr.Keys != nil {
			out[i].Keys, _ = json.Marshal(*pr.Keys)
		}
	}
	return out
}

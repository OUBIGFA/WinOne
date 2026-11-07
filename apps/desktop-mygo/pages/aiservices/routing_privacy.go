package aiservices

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Mask presentation only. Request IDs, account restrictions and unrest keys
// keep their exact upstream values when an action is performed.
func (r *routingPanel) accountMasker() func(string) string {
	if r.p.draft == nil || !r.p.draft.Value.magpie().HideAccounts {
		return func(s string) string { return s }
	}
	identities := map[string]string{}
	add := func(id, name string) {
		if id != "" {
			identities[id] = name
		}
	}
	for _, tally := range r.trace.tallies {
		if tally.Who != "" {
			add(tally.ID, tally.Who)
		}
	}
	if r.state != nil {
		for _, accounts := range r.state.Accounts {
			for _, a := range accounts {
				add(a.ID, a.Name)
			}
		}
		for _, pool := range r.state.Pools {
			for _, who := range pool.Who {
				add(who, who)
			}
		}
	}
	rows := r.trace.routes
	if r.trace.selected != nil {
		rows = append(slices.Clone(rows), *r.trace.selected)
	}
	for _, row := range rows {
		var evidence traceEvidence
		if json.Unmarshal(row.Raw, &evidence) != nil {
			continue
		}
		for _, candidate := range append(evidence.Order, evidence.Left...) {
			if candidate.Who != "" {
				add(candidate.ID, candidate.Who)
			}
		}
	}
	replacements := map[string]string{}
	for index, id := range sortedKeys(identities) {
		label := i18n.T("usage.maskedAccount", "number", index+1)
		replacements[id] = label
		if name := identities[id]; name != "" {
			replacements[name] = label
		}
	}
	keys := sortedKeys(replacements)
	// Replace a full provider/account ID before its contained account name.
	slices.SortStableFunc(keys, func(a, b string) int { return len(b) - len(a) })
	var pairs []string
	for _, key := range keys {
		pairs = append(pairs, key, replacements[key])
	}
	return strings.NewReplacer(pairs...).Replace
}

func routingJSON(c *ui.Context, value any, mask func(string) string) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		kit.Mono(c, mask(string(data))).FillWidth().MinWidth(0).Selectable()
	}
}

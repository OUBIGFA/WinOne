package aiservices

import (
	"regexp"
	"strings"

	"winone/desktopmygo/kit"
)

var internalRouteID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/?`)

// Resolve display names at the presentation boundary. References, including
// reasoning suffixes and plugin namespaces, are never rewritten for requests.
func (r *routingPanel) memberLabel(ref string) string {
	var best routeModel
	consider := func(m routeModel) {
		if m.ID != "" && (ref == m.ID || strings.HasPrefix(ref, m.ID+":")) && len(m.ID) > len(best.ID) {
			best = m
		}
	}
	if r.state != nil {
		for _, list := range [][]routeModel{r.state.Models, r.state.Drawers, r.state.Deciders} {
			for _, model := range list {
				consider(model)
			}
		}
		for _, group := range r.state.Groups {
			for _, model := range group.Value.MemberInfo {
				consider(model)
			}
			consider(routeModel{ID: group.Value.Ref, Name: group.Value.Name, ProviderName: nativeT("groups")})
		}
	}
	name := ref
	if best.ID != "" {
		name = best.Name
		if name == "" {
			_, name, _ = strings.Cut(best.ID, "/")
		}
		name += strings.TrimPrefix(ref, best.ID)
		if best.ProviderName != "" {
			name += " · " + best.ProviderName
		}
	} else if providerID, model, ok := strings.Cut(ref, "/"); ok {
		name = model
		if provider := r.providerLabel(providerID); provider != "" {
			name += " · " + provider
		}
	}
	name = strings.TrimSpace(internalRouteID.ReplaceAllString(name, ""))
	if name == "" {
		return nativeT("unavailableModel")
	}
	return name
}

func (r *routingPanel) providerLabel(id string) string {
	if r.p != nil && r.p.draft != nil {
		for _, doc := range r.p.draft.Value.Providers {
			if doc.Value.ID == id {
				return doc.Value.Name
			}
		}
	}
	if r.state != nil {
		for _, m := range r.state.Models {
			if m.Provider == id && m.ProviderName != "" {
				return m.ProviderName
			}
		}
		for _, pool := range r.state.Pools {
			if pool.Provider == id {
				return pool.Name
			}
		}
	}
	return strings.TrimSpace(internalRouteID.ReplaceAllString(id, ""))
}

func (r *routingPanel) memberChoices(refs []string) []kit.SelectOption {
	choices := make([]kit.SelectOption, 0, len(refs))
	for _, ref := range refs {
		choices = append(choices, kit.SelectOption{Value: ref, Label: r.memberLabel(ref)})
	}
	return choices
}

func (r *routingPanel) memberNames(refs []string) string {
	labels := make([]string, 0, len(refs))
	for _, ref := range refs {
		labels = append(labels, r.memberLabel(ref))
	}
	return strings.Join(labels, " · ")
}

func (r *routingPanel) strategyLabel(strategy string) string {
	for _, choice := range r.routingChoices(strategy) {
		if choice.Value == strategy {
			return choice.Label
		}
	}
	return strategy
}

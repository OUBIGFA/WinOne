package ai

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"
)

// ModelOption is the public vendor catalog entry, shared by refresh and UI.
type ModelOption struct {
	Metadata ModelMetadata `json:"metadata,omitempty"`
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Icon     string        `json:"icon,omitempty"`
	Free     bool          `json:"free,omitempty"`
}

// RefreshModels keeps explicit edits over a changing vendor catalog. The first
// refresh of a legacy configuration preserves unknown/manual entries because
// older versions did not record their origin. Empty vendor replies are transient.
func (p *Provider) RefreshModels(models []ModelOption) {
	if len(models) == 0 {
		return
	}
	p.separateModelDefaults()
	old := p.AllModels()
	listed := make([]string, 0, len(models))
	availableIDs := make(map[string]bool, len(models)+len(p.ManualModels))
	for _, m := range models {
		if !availableIDs[m.ID] {
			listed = append(listed, m.ID)
			availableIDs[m.ID] = true
		}
	}
	manualIDs := make(map[string]bool, len(p.ManualModels))
	for _, id := range p.ManualModels {
		manualIDs[id] = true
	}
	if p.CatalogModels == nil {
		for _, id := range old {
			if !availableIDs[id] && !manualIDs[id] {
				p.ManualModels = append(p.ManualModels, id)
				manualIDs[id] = true
			}
		}
	}
	available := slices.Clone(listed)
	for _, id := range p.ManualModels {
		if !availableIDs[id] {
			available = append(available, id)
			availableIDs[id] = true
		}
	}
	names, free, metadata, baseline := map[string]string{}, map[string]bool{}, map[string]ModelMetadata{}, map[string]ModelMetadata{}
	catalogNames := map[string]string{}
	for _, id := range p.ManualModels {
		if name := p.ModelNames[id]; name != "" {
			names[id] = name
		}
		free[id], metadata[id] = p.ModelFree[id], p.ModelMetadata[id].clone()
	}
	for _, m := range models {
		catalogNames[m.ID] = m.Name
		if m.Name != "" {
			names[m.ID] = m.Name
		}
		if name := p.ModelNames[m.ID]; name != "" {
			previous, known := p.CatalogNames[m.ID]
			if known && name != previous || !known && name != m.ID {
				names[m.ID] = name
			}
		}
		free[m.ID], baseline[m.ID] = m.Free, m.Metadata.clone()
		if current, ok := p.ModelMetadata[m.ID]; ok {
			metadata[m.ID] = current.clone()
		}
	}
	picks, delisted := []string{}, []string{}
	seenPicks := make(map[string]bool, len(p.Models)+len(p.Delisted))
	for _, id := range append(slices.Clone(p.Models), p.Delisted...) {
		if seenPicks[id] {
			continue
		}
		seenPicks[id] = true
		if availableIDs[id] {
			picks = append(picks, id)
		} else {
			delisted = append(delisted, id)
			metadata[id] = p.ModelMetadata[id].clone()
			if name := p.ModelNames[id]; name != "" {
				names[id] = name
			}
		}
	}
	p.Models = IncludeDiscovered(picks, old, available, p.AutoInclude)
	p.AvailableModels, p.Delisted, p.ModelNames, p.ModelFree, p.ModelMetadata = available, delisted, names, free, metadata
	p.CatalogModels, p.CatalogMetadata = listed, baseline
	p.CatalogNames = catalogNames
}

// Older versions wrote the vendor report and the user's edits into one map.
// Migrate once, against the report retained alongside it. Unknown legacy
// values remain explicit: without a report there is no evidence to erase them.
func (p *Provider) separateModelDefaults() {
	if p.ModelDefaultsSeparated {
		return
	}
	for id, current := range p.ModelMetadata {
		if previous, known := p.CatalogMetadata[id]; known {
			p.ModelMetadata[id] = catalogOverrides(previous, current)
		}
	}
	p.ModelDefaultsSeparated = true
}

func catalogOverrides(previous, current ModelMetadata) ModelMetadata {
	decode := func(m ModelMetadata) map[string]json.RawMessage {
		b, _ := json.Marshal(m)
		var v map[string]json.RawMessage
		_ = json.Unmarshal(b, &v)
		return v
	}
	a, b, c := decode(previous), decode(current), map[string]json.RawMessage{}
	for key, now := range b {
		if !reflect.DeepEqual(a[key], now) {
			c[key] = now
		}
	}
	raw, _ := json.Marshal(c)
	var result ModelMetadata
	_ = json.Unmarshal(raw, &result)
	return result
}

// ModelRefreshInterval follows upstream's explicit opt-in and remote gateway
// default, and account/plugin relisting and recovery intervals.
func (p Provider) ModelRefreshInterval() time.Duration {
	if p.Off || p.ListOff || p.Protocol == "decide" {
		return 0
	}
	if p.AuthMode == "account" {
		// Match upstream's hourly account/plugin relist and quicker recovery
		// when a temporarily missing model has a paused selection.
		if len(p.Models) == 0 && len(p.Delisted) == 0 {
			return 0
		}
		if len(p.Delisted) > 0 {
			return 5 * time.Minute
		}
		return time.Hour
	}
	if p.AuthMode != "api-key" {
		return 0
	}
	every := p.RefreshEvery
	if strings.TrimSpace(every) == "" && p.Preset == "remote-magpie" {
		every = "10m"
	}
	d, _ := ParseRefreshEvery(every)
	return d
}

// IncludeDiscovered matches only newly listed IDs. Re-fetching the same list
// must not re-enable a model the user deliberately unchecked after a refresh.
// On a rule edit the caller passes no old list, as upstream SetAutoInclude does.
func IncludeDiscovered(selected, old, listed, rules []string) []string {
	out := slices.Clone(selected)
	if len(listed) == 0 || len(rules) == 0 {
		return out
	}
	// Old and already selected IDs are membership checks, not ordered searches.
	// Keep the output order and never re-enable a deliberately unchecked model.
	seen := make(map[string]struct{}, len(old)+len(selected))
	for _, id := range old {
		seen[id] = struct{}{}
	}
	for _, id := range selected {
		seen[id] = struct{}{}
	}
	// Compile each upstream rule once for this catalog, rather than once per
	// candidate. Invalid rules keep the existing non-matching behavior.
	var compiled []matcher
	for _, id := range listed {
		if _, exists := seen[id]; exists {
			continue
		}
		if compiled == nil {
			compiled = make([]matcher, 0, len(rules))
			for _, pattern := range rules {
				if rule, err := compileRule(pattern); err == nil {
					compiled = append(compiled, rule.ids)
				}
			}
		}
		if slices.ContainsFunc(compiled, func(rule matcher) bool { return rule.MatchString(id) }) {
			out = append(out, id)
			seen[id] = struct{}{}
		}
	}
	return out
}

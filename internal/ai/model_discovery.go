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
	old := p.AllModels()
	listed := make([]string, 0, len(models))
	for _, m := range models {
		if !slices.Contains(listed, m.ID) {
			listed = append(listed, m.ID)
		}
	}
	if p.CatalogModels == nil {
		for _, id := range old {
			if !slices.Contains(listed, id) && !slices.Contains(p.ManualModels, id) {
				p.ManualModels = append(p.ManualModels, id)
			}
		}
	}
	available := slices.Clone(listed)
	for _, id := range p.ManualModels {
		if !slices.Contains(available, id) {
			available = append(available, id)
		}
	}
	names, free, metadata, baseline := map[string]string{}, map[string]bool{}, map[string]ModelMetadata{}, map[string]ModelMetadata{}
	catalogNames := map[string]string{}
	for _, id := range p.ManualModels {
		if name := p.ModelNames[id]; name != "" {
			names[id] = name
		}
		free[id], metadata[id] = p.ModelFree[id], p.ModelMetadata[id]
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
		metadata[m.ID] = m.Metadata.clone()
		if current, ok := p.ModelMetadata[m.ID]; ok {
			if previous, known := p.CatalogMetadata[m.ID]; known {
				metadata[m.ID] = mergeCatalogMetadata(previous, current, m.Metadata)
			} else {
				metadata[m.ID] = current.clone()
			}
		}
	}
	picks, delisted := []string{}, []string{}
	for _, id := range append(slices.Clone(p.Models), p.Delisted...) {
		if slices.Contains(available, id) {
			if !slices.Contains(picks, id) {
				picks = append(picks, id)
			}
		} else if !slices.Contains(delisted, id) {
			delisted = append(delisted, id)
			metadata[id] = p.ModelMetadata[id]
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

// Compare individual fields with the previous report: only fields the user
// changed survive the next report, including explicit resets/removals.
func mergeCatalogMetadata(previous, current, next ModelMetadata) ModelMetadata {
	decode := func(m ModelMetadata) map[string]json.RawMessage {
		b, _ := json.Marshal(m)
		var v map[string]json.RawMessage
		_ = json.Unmarshal(b, &v)
		return v
	}
	a, b, c := decode(previous), decode(current), decode(next)
	for key, now := range b {
		if !reflect.DeepEqual(a[key], now) {
			c[key] = now
		}
	}
	for key := range a {
		if _, exists := b[key]; !exists {
			delete(c, key)
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
	for _, id := range listed {
		if slices.Contains(old, id) || slices.Contains(out, id) {
			continue
		}
		if slices.ContainsFunc(rules, func(rule string) bool { return PatternMatches(rule, id) }) {
			out = append(out, id)
		}
	}
	return out
}

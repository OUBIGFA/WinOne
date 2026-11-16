package ai

import "slices"

// Replay a newer vendor report through the existing discovery rules before
// comparing user edits. This preserves renames, manual models and explicit
// overrides, and pauses newly selected models which were delisted while the
// page was open. Deleting a provider also remains possible after a refresh.
func mergeCatalogSelections(base Configuration, draft *Configuration, current Configuration) {
	for i := range base.Providers {
		before := &base.Providers[i]
		now, exists := current.Provider(before.ID)
		if !exists {
			continue
		}
		var next *Provider
		if at := slices.IndexFunc(draft.Providers, func(p Provider) bool { return p.ID == before.ID }); at >= 0 {
			next = &draft.Providers[at]
		}
		previousRefresh, previousError := before.ModelRefreshAt, before.ModelRefreshError
		if sameCatalogSource(*before, now) {
			if !previousRefresh.Equal(now.ModelRefreshAt) && now.CatalogModels != nil {
				models := make([]ModelOption, 0, len(now.CatalogModels))
				for _, id := range now.CatalogModels {
					models = append(models, ModelOption{ID: id, Name: now.CatalogNames[id], Metadata: now.CatalogMetadata[id], Free: now.ModelFree[id]})
				}
				if next != nil && next.ModelRefreshAt.Equal(previousRefresh) && sameCatalogSource(*before, *next) {
					next.RefreshModels(models)
				}
				before.RefreshModels(models)
			}
			before.ModelRefreshAt, before.ModelRefreshError = now.ModelRefreshAt, now.ModelRefreshError
			if next != nil {
				if next.ModelRefreshAt.Equal(previousRefresh) {
					next.ModelRefreshAt = now.ModelRefreshAt
				}
				if next.ModelRefreshError == previousError {
					next.ModelRefreshError = now.ModelRefreshError
				}
			}
		}
		if next == nil {
			continue
		}
		// Membership is a set with a display order. Independent additions or
		// removals by another editor can coexist without restoring old picks.
		for _, field := range []struct {
			base, draft *[]string
			current     []string
		}{
			{&before.Models, &next.Models, now.Models},
			{&before.AvailableModels, &next.AvailableModels, now.AvailableModels},
			{&before.ManualModels, &next.ManualModels, now.ManualModels},
			{&before.Delisted, &next.Delisted, now.Delisted},
		} {
			if slices.Equal(*field.base, *field.draft) || slices.Equal(*field.base, field.current) {
				continue
			}
			merged := slices.Clone(*field.draft)
			for _, id := range field.current {
				if !slices.Contains(*field.base, id) && !slices.Contains(merged, id) {
					merged = append(merged, id)
				}
			}
			merged = slices.DeleteFunc(merged, func(id string) bool {
				return slices.Contains(*field.base, id) && !slices.Contains(field.current, id)
			})
			*field.base, *field.draft = slices.Clone(field.current), merged
		}
	}
}

func sameCatalogSource(a, b Provider) bool {
	return a.AuthMode == b.AuthMode && a.BaseURL == b.BaseURL && a.Protocol == b.Protocol && a.Agent == b.Agent && a.AgentBackend == b.AgentBackend
}

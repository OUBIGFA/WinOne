package aiservices

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// Selection is local to each provider tab. The dialog's final action changes
// only the shared draft; the existing save still commits the whole edit once.
func (p *page) providerBulkDialog(c *ui.Context) {
	if !p.bulkProvidersOpen || p.draft == nil || p.tab != tabCustom && p.tab != tabAccounts {
		return
	}
	if p.bulkProviderSelection == nil {
		p.bulkProviderSelection = map[int]map[string]bool{}
		p.bulkProviderSearch = map[int]string{}
	}
	selected := p.bulkProviderSelection[p.tab]
	if selected == nil {
		selected = map[string]bool{}
		p.bulkProviderSelection[p.tab] = selected
	}
	providers := p.sectionProviders()
	count := 0
	for _, pr := range providers {
		if selected[pr.ID] {
			count++
		}
	}
	query := p.bulkProviderSearch[p.tab]
	title := "aiBulkProviders.titleCustom"
	if p.tab == tabAccounts {
		title = "aiBulkProviders.titleAccounts"
	}
	locked := p.locked()
	confirmed := kit.Dialog(c, &p.bulkProvidersOpen, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T("aiBulkProviders.hint"),
		Confirm: i18n.T("aiBulkProviders.remove"), Cancel: i18n.T("common.cancel"), Danger: true,
		ConfirmDisabled: locked || count == 0, Busy: locked, Width: 560,
		Note: i18n.T("aiBulkProviders.selected", "count", count, "total", len(providers)),
		Header: func() {
			kit.TextInput(c, &query).FillWidth().Label(i18n.T("aiBulkProviders.search")).Placeholder(i18n.T("aiBulkProviders.search")).Disabled(locked)
			p.bulkProviderSearch[p.tab] = query
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if kit.Button(c, i18n.T("aiBulkProviders.all"), icons.Check, false).Disabled(locked || count == len(providers)).Clicked() {
					for _, pr := range providers {
						selected[pr.ID] = true
					}
				}
				if kit.Button(c, i18n.T("aiBulkProviders.none"), icons.X, false).Disabled(locked || count == 0).Clicked() {
					clear(selected)
				}
			})
		},
		Body: func() {
			query := strings.ToLower(strings.TrimSpace(query))
			shown := 0
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for index, pr := range providers {
					name := pr.Name
					if strings.TrimSpace(name) == "" {
						name = i18n.T("aiBulkProviders.unnamed", "number", index+1)
					}
					if !strings.Contains(strings.ToLower(name), query) {
						continue
					}
					shown++
					ui.Row(c.Key(pr.ID)).FillWidth().MinHeight(40).Gap(10).Padding(8, 4).AlignItems(ui.Center).Children(func() {
						widgets.ProviderIcon(c, p.env, pr.Icon, 20)
						on := selected[pr.ID]
						if ui.Checkbox(c, &on, name).Grow(1).MinWidth(0).Label(i18n.T("aiBulkProviders.pick", "name", name)).Disabled(locked).Changed() {
							if on {
								selected[pr.ID] = true
							} else {
								delete(selected, pr.ID)
							}
						}
						kit.Caption(c, i18n.T("aiBulkProviders.models", "count", len(pr.Models)))
					})
				}
			})
			if shown == 0 {
				kit.Help(c, i18n.T("aiBulkProviders.noMatch"))
			}
		},
	})
	if !confirmed {
		return
	}
	// Snapshot the selected IDs before remove compacts the provider slice.
	var ids []string
	keep := ""
	if current := p.current(); current != nil {
		keep = current.ID
	}
	for _, pr := range providers {
		if selected[pr.ID] {
			ids = append(ids, pr.ID)
		}
	}
	for _, id := range ids {
		p.remove(id)
		delete(selected, id)
	}
	if p.draft.Value.find(keep) != nil {
		p.selected = keep
	}
	p.bulkProvidersOpen = false
}

package aiservices

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

func searchKeySlots(value string) []ai.SearchKey {
	var keys []ai.SearchKey
	// Keep the upstream separators when converting an existing key string.
	for _, key := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		key = strings.TrimSpace(key)
		if key != "" && !slices.ContainsFunc(keys, func(k ai.SearchKey) bool { return k.Key == key }) {
			keys = append(keys, ai.SearchKey{ID: newID(), Key: key})
		}
	}
	return keys
}

// Legacy keys are revealed only by the explicit management action. Thereafter
// public rows contain slot IDs; Show/Copy fetch only the chosen credential.
func (a *advancedPanel) searchKeys(c *ui.Context, api searchAPI, locked bool) {
	if api.KeyList == nil {
		if (api.HasKey || api.Key != "") && kit.Button(c, nativeT("manageSearchKeys"), icons.KeyRound, false).Disabled(locked).Clicked() {
			a.readSearchKey(api, func(value string) {
				keys := searchKeySlots(value)
				a.editSearchAPI(api.Vendor, func(v *searchAPI) {
					// A second click must not overwrite edits made after the first reply.
					if v.KeyList == nil && v.Key == api.Key {
						v.KeyList, v.Key = &keys, ""
					}
				})
			})
		}
		return
	}
	edit := func(change func(*[]ai.SearchKey)) {
		a.editSearchAPI(api.Vendor, func(v *searchAPI) { keys := slices.Clone(*v.KeyList); change(&keys); v.KeyList = &keys })
	}
	if len(*api.KeyList) == 0 {
		kit.Help(c, nativeT("noSearchKeys"))
	}
	for index, slot := range slices.Clone(*api.KeyList) {
		ref := api.Vendor + "/" + slot.ID
		name := i18n.T("aiNative.searchKeyNumber", "number", index+1)
		ui.Column(c.Key(slot.ID)).FillWidth().Gap(5).Padding(8, 0).Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				on := !slot.Off
				if ui.Checkbox(c, &on, name).Grow(1).Disabled(locked).Changed() {
					edit(func(keys *[]ai.SearchKey) { (*keys)[index].Off = !on })
				}
				if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")+" "+name).Disabled(locked || index == 0).Clicked() {
					edit(func(keys *[]ai.SearchKey) { (*keys)[index], (*keys)[index-1] = (*keys)[index-1], (*keys)[index] })
				}
				if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")+" "+name).Disabled(locked || index == len(*api.KeyList)-1).Clicked() {
					edit(func(keys *[]ai.SearchKey) { (*keys)[index], (*keys)[index+1] = (*keys)[index+1], (*keys)[index] })
				}
				read := func(done func(string)) {
					if slot.Key != "" {
						done(slot.Key)
					} else {
						a.readSearchKey(searchAPI{Vendor: ref, HasKey: slot.HasKey}, done)
					}
				}
				label := i18n.T("ai.showSearchKey", "name", name)
				if a.revealed[ref] != "" {
					label = i18n.T("ai.hideSearchKey", "name", name)
				}
				if kit.IconButton(c, icons.Eye, label).Disabled(locked).Clicked() {
					if a.revealed[ref] != "" {
						delete(a.revealed, ref)
					} else {
						read(func(key string) { a.revealed[ref] = key })
					}
				}
				if kit.IconButton(c, icons.Copy, i18n.T("ai.copySearchKey", "name", name)).Disabled(locked).Clicked() {
					services := c.Services()
					read(func(key string) { services.WriteClipboard(key) })
				}
				if kit.IconButton(c, icons.Trash2, nativeT("remove")+" "+name).Disabled(locked).Clicked() {
					edit(func(keys *[]ai.SearchKey) { *keys = slices.Delete(*keys, index, index+1) })
					delete(a.revealed, ref)
				}
			})
			if key := a.revealed[ref]; key != "" {
				kit.Help(c, key).Selectable()
			}
		})
	}
}

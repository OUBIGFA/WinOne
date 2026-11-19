package aiservices

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

// Read saved keys once and combine only the selected provider's current draft.
// Copies include disabled keys, in list order, as Magpie AllKeys does.
func (e *editor) withKeySnapshot(pr *provider, done func(map[string]string)) {
	e.bulkKeysBusy, e.bulkKeysError = true, ""
	p, id := e.p, pr.ID
	saved := p.saved != nil && p.saved.Value.find(id) != nil
	core.Go(p.env, func() (map[string]string, error) {
		keys := map[string]string{}
		if saved {
			var reply struct {
				Keys map[string]string `json:"keys"`
			}
			if err := p.env.Host.CallCompletingInto("capture.ai.keys.reveal", map[string]any{"provider": id, "all": true}, &reply); err != nil {
				return nil, err
			}
			keys = reply.Keys
		}
		return keys, nil
	}, func(keys map[string]string, err error) {
		e.bulkKeysBusy = false
		if p.editor != e || e.closed || p.draft == nil {
			return
		}
		if err != nil {
			e.bulkKeysError = err.Error()
			return
		}
		if keys == nil {
			keys = map[string]string{}
		}
		for slot, key := range p.keys[id] {
			keys[slot] = key
		}
		done(keys)
	})
}

func (e *editor) bulkKeys(c *ui.Context, pr *provider, locked bool) {
	busy := locked || e.bulkKeysBusy
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.GhostButton(c, i18n.T("aiSync.pasteKeys"), icons.ClipboardPaste, false).Disabled(busy).Clicked() {
			e.bulkKeysOpen = true
			e.bulkKeysError = ""
		}
		services := c.Services()
		if kit.GhostButton(c, i18n.T("aiSync.copyKeys"), icons.Copy, false).Disabled(busy).Clicked() {
			e.withKeySnapshot(pr, func(keys map[string]string) {
				var lines []string
				seen := map[string]bool{}
				for _, slot := range pr.slots() {
					key := strings.TrimSpace(keys[slot.ID])
					if key != "" && !seen[key] {
						seen[key] = true
						lines = append(lines, key)
					}
				}
				services.WriteClipboard(strings.Join(lines, "\n"))
				e.p.env.Toast(i18n.T("aiSync.keysCopied", "count", len(lines)))
			})
		}
	})
	kit.InlineError(c, e.bulkKeysError)
	if !e.bulkKeysOpen {
		return
	}
	confirmed := kit.Dialog(c, &e.bulkKeysOpen, kit.DialogOptions{
		Title: i18n.T("aiSync.pasteKeys"), Confirm: i18n.T("aiSync.addKeys"), Cancel: i18n.T("common.cancel"), Busy: e.bulkKeysBusy, Error: e.bulkKeysError,
		ConfirmDisabled: busy || len(ai.SplitKeys(e.bulkKeysText)) == 0,
		Body: func() {
			kit.Help(c, i18n.T("aiSync.pasteKeysHint"))
			ui.TextArea(c, &e.bulkKeysText).FillWidth().Lines(4, 10).Label(i18n.T("aiSync.pasteKeys")).Disabled(busy)
		},
	})
	if confirmed {
		incoming := ai.SplitKeys(e.bulkKeysText)
		e.withKeySnapshot(pr, func(keys map[string]string) {
			seen := map[string]bool{}
			for _, slot := range pr.slots() {
				if key := strings.TrimSpace(keys[slot.ID]); key != "" {
					seen[key] = true
				}
			}
			slots := append([]keySlot{}, pr.slots()...)
			added := 0
			for _, key := range incoming {
				if seen[key] {
					continue
				}
				seen[key] = true
				id := ""
				for _, slot := range slots {
					if !slot.HasKey && keys[slot.ID] == "" {
						id = slot.ID
						break
					}
				}
				if id == "" {
					id = newID()
					slots = append(slots, keySlot{ID: id, Enabled: true})
				}
				keys[id] = key
				e.p.changeKey(pr.ID, id, &key)
				added++
			}
			if added > 0 {
				e.p.change(pr.ID, func(target *provider) { target.Keys = &slots })
				e.bulkKeysText = ""
				e.bulkKeysOpen = false
			} else {
				e.bulkKeysError = i18n.T("aiSync.keysAlreadyPresent")
				e.bulkKeysOpen = true
			}
		})
	}
}

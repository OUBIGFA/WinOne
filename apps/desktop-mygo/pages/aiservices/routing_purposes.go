package aiservices

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Keep Magpie's purpose aliases together. Unknown kinds stay distinct from
// unmarked requests, and selecting several purposes uses OR semantics.
func routePurpose(kind string) string {
	switch kind {
	case "":
		return "unmarked"
	case "auto_review", "guardian_review":
		kind = "guardian"
	case "memgen", "memory":
		kind = "memory_consolidation"
	case "thread_title_reconsideration", "title_generation", "title":
		kind = "thread_title"
	case "thread_spawn", "agent_job":
		kind = "collab_spawn"
	case "ambient_suggestion_safety":
		kind = "ambient_suggestions"
	}
	return "kind:" + kind
}

func routePurposeLabel(purpose string) string {
	id := strings.TrimPrefix(purpose, "kind:")
	key := "aiRouting.purpose." + id
	if label := i18n.T(key); label != key {
		return label
	}
	return id
}

func (t *nativeTrace) purposeView(c *ui.Context) {
	if t.purposes == nil {
		t.purposes = map[string]bool{}
	}
	available := map[string]bool{}
	for _, row := range t.routes {
		available[routePurpose(row.Kind)] = true
	}
	for id := range t.purposes {
		available[id] = true
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		kit.Label(c, i18n.T("aiRouting.purpose"))
		ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
			if kit.GhostButton(c, nativeT("all"), nil, false).Clicked() {
				clear(t.purposes)
			}
			for _, id := range sortedKeys(available) {
				on := t.purposes[id]
				if ui.Checkbox(c.Key(id), &on, routePurposeLabel(id)).Changed() {
					if on {
						t.purposes[id] = true
					} else {
						delete(t.purposes, id)
					}
				}
			}
		})
	})
}

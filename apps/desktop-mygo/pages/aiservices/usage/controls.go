package usage

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// field is a labeled control in a row of filters: it takes its share of
// the row and goes below the others when the row is narrow.
func field(c *ui.Context, label string, fn func()) ui.Element {
	return ui.Field(c, label, fn).Basis(200).Grow(1).Shrink(1).MinWidth(0)
}

// choose is kit.Select across its field, which names it for assistive
// technology. It reports a new choice.
func choose(c *ui.Context, value *string, options []kit.SelectOption) bool {
	changed, trigger := kit.Select(c, value, options)
	trigger.FillWidth().MinWidth(0)
	return changed
}

// pickItem is one option of pickSelect; icon is a provider's mark.
type pickItem struct {
	value, label, icon string
}

// pickSelect is choose for options marked with a provider's picture, as
// React's provider filter showed them (widgets' model pickers draw their
// marks the same way); its field names it. It reports a new choice.
func pickSelect(c *ui.Context, selected *string, items []pickItem, env *core.Env) bool {
	t, p := c.Theme(), kit.Colors(c)
	before := *selected
	sel := ui.SelectBase(c, selected)
	var current *pickItem
	for i := range items {
		if items[i].value == *selected {
			current = &items[i]
		}
	}
	mark := func(item *pickItem) {
		if item.icon != "" || item.value != "" {
			widgets.ProviderIcon(c, env, item.icon, 16)
		}
	}
	b := sel.Trigger.FillWidth().MinHeight(kit.ControlHeight).Padding(5, 10).Gap(8).Radius(t.Radius).Background(t.Surface).
		Border(1, t.Border).Justify(ui.SpaceBetween).AlignItems(ui.Center).MinWidth(0)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	shown := *selected
	if current != nil {
		shown = current.label
	}
	b.Tooltip(shown).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			if current != nil {
				mark(current)
			}
			ui.Text(c, shown).SingleLine().Shrink(1).MinWidth(0)
		})
		ui.Icon(c, icons.ChevronDown).FontSize(14).TextColor(t.TextMuted).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(kit.PanelRadius).Background(p.Panel).Border(1, t.Border).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, popupShadow(t))).MinWidth(200).MaxWidth(420)
		for i := range items {
			item := &items[i]
			row := sel.Item(item.value).Padding(6, 10).Gap(8).Radius(t.Radius).AlignItems(ui.Center)
			if row.Highlighted() {
				row.Background(t.SurfaceHover)
			}
			chosen := item.value == *selected
			row.Children(func() {
				mark(item)
				label := ui.Text(c, item.label).Grow(1).Shrink(1).MinWidth(0)
				if chosen {
					label.FontWeight(600)
					ui.Icon(c, icons.Check).FontSize(14).Shrink(0)
				}
			})
		}
	})
	return *selected != before
}

// popupShadow is the alpha of a popup's shadow, as kit's own popups use.
func popupShadow(t *ui.Theme) float32 {
	if t.Dark {
		return 0.7
	}
	return 0.16
}

// pager is React's UsagePager: where the page is among total, and the
// previous and next fifteen. It shows nothing when there is nothing.
func pager(c *ui.Context, total, offset, count int, loading bool, change func(offset int)) {
	if total == 0 {
		return
	}
	ui.Row(c).FillWidth().GapX(12).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
		from, to := min(offset+1, total), min(offset+count, total)
		kit.Help(c, tr("usage.page", "from", from, "to", to, "total", total))
		ui.Row(c).Gap(8).Children(func() {
			if kit.Button(c, tr("usage.previous"), icons.ChevronLeft, false).Disabled(loading || offset == 0).Clicked() {
				change(max(0, offset-pageSize))
			}
			if kit.Button(c, tr("usage.next"), icons.ChevronRight, false).Disabled(loading || offset+pageSize >= total).Clicked() {
				change(offset + pageSize)
			}
		})
	})
}

// pageSize is how many requests or sessions a page lists.
const pageSize = 15

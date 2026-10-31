package usage

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// aliasSource is the aliases' entry in the page's activity: their form
// while it holds unapplied text, and an edit the Host checks.
const aliasSource = "ai-usage-aliases"

// aliasEditor is React's UsageAliases form: a model's identifier and the
// name it is merged under. Magpie's alias setter checks the spelling,
// the chains and cycles; a checked edit goes into the page's draft, which
// Save keeps.
type aliasEditor struct {
	from, to string
	busy     bool
	err      string
}

// reset forgets the form, as React unmounted it.
func (a *aliasEditor) reset() { a.from, a.to, a.err = "", "", "" }

// reportAliases tells the page about the form: unapplied text is unsaved
// input, which the page's guard asks about, and discarding clears it.
func (u *Usage) reportAliases() {
	if u.closed {
		return
	}
	a := &u.aliases
	u.env.Activity.Report(aliasSource, a.from != "" || a.to != "", a.busy, func() {
		a.reset()
		u.env.Activity.Report(aliasSource, false, a.busy, nil)
	})
}

// applyAlias has Magpie check the whole draft with one edit (target ""
// removes source's alias) and writes the result into the page's draft.
// A failed check leaves the draft and the form as they were.
func (u *Usage) applyAlias(source, target string, clearForm bool) {
	a := &u.aliases
	if a.busy || u.opts.Disabled || u.opts.Change == nil {
		return
	}
	a.busy, a.err = true, ""
	u.reportAliases()
	aliases := map[string]string{}
	if p := u.opts.Preferences; p != nil && p.Aliases != nil {
		aliases = p.Aliases
	}
	params := map[string]any{"aliases": aliases, "edit": map[string]string{"from": source, "to": target}}
	core.Go(u.env, func() (map[string]string, error) {
		var reply struct {
			UsageAliases map[string]string `json:"usageAliases"`
		}
		err := u.env.Host.CallCompletingInto("capture.ai.magpie.usage-aliases", params, &reply)
		if err == nil && reply.UsageAliases == nil {
			reply.UsageAliases = map[string]string{}
		}
		return reply.UsageAliases, err
	}, func(result map[string]string, err error) {
		if u.closed {
			return
		}
		a.busy = false
		if err != nil {
			a.err = err.Error()
		} else {
			if change := u.opts.Change; change != nil {
				var next Preferences
				if p := u.opts.Preferences; p != nil {
					next = *p
				}
				next.Aliases = result
				change(next)
			}
			if clearForm {
				a.from, a.to = "", ""
			}
		}
		u.reportAliases()
	})
}

// aliasesView is the aliases: the form, the rule that Save keeps them,
// and the aliases with their removal.
func (u *Usage) aliasesView(c *ui.Context) {
	a := &u.aliases
	disabled := u.opts.Disabled
	locked := a.busy || disabled
	valid := strings.TrimSpace(a.from) != "" && strings.TrimSpace(a.to) != ""
	var aliases map[string]string
	if p := u.opts.Preferences; p != nil {
		aliases = p.Aliases
	}
	ui.Column(c).FillWidth().Gap(16).Role(ui.RoleGroup).Label(tr("usage.aliases")).Children(func() {
		kit.Help(c, tr("usage.aliasHint"))
		submitted := false
		ui.Row(c).FillWidth().GapX(12).GapY(12).Wrap().AlignItems(ui.End).Children(func() {
			field(c, tr("usage.aliasFrom"), func() {
				in := ui.TextInput(c.Key("from"), &a.from).FillWidth().Placeholder("deepseek-v4.1-flash").Label(tr("usage.aliasFrom")).Disabled(locked)
				submitted = submitted || in.Submitted()
			})
			field(c, tr("usage.aliasTo"), func() {
				in := ui.TextInput(c.Key("to"), &a.to).FillWidth().Placeholder("deepseek-flash").Label(tr("usage.aliasTo")).Disabled(locked)
				submitted = submitted || in.Submitted()
			})
			if kit.Button(c, tr("usage.aliasApply"), icons.Plus, a.busy).Disabled(locked || !valid).Shrink(0).Clicked() {
				submitted = true
			}
		})
		if submitted && valid && !locked {
			u.applyAlias(a.from, a.to, true)
		}
		if a.err != "" {
			kit.Notice(c, kit.Failure, "", a.err, nil)
		}
		kit.Help(c, tr("usage.aliasSaveHint"))
		if len(aliases) == 0 {
			kit.Help(c, tr("usage.aliasEmpty"))
			return
		}
		sources := make([]string, 0, len(aliases))
		for source := range aliases {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		t, p := c.Theme(), kit.Colors(c)
		ui.Column(c).FillWidth().Role(ui.RoleList).Children(func() {
			for _, source := range sources {
				ui.Row(c.Key(source)).FillWidth().Gap(12).Padding(10, 0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
					BorderColor(p.Hairline).Role(ui.RoleListItem).Children(func() {
					ui.Row(c).Grow(1).Shrink(1).MinWidth(0).GapX(8).GapY(4).Wrap().AlignItems(ui.Center).Children(func() {
						ui.Text(c, source).Selectable().MinWidth(0)
						ui.Icon(c, icons.ArrowRight).FontSize(16).TextColor(t.TextMuted).Shrink(0)
						ui.Text(c, aliases[source]).Selectable().MinWidth(0)
					})
					if kit.GhostButton(c, tr("usage.aliasRemove"), icons.Trash2, false).Label(tr("usage.aliasRemoveNamed", "name", source)).
						Disabled(locked).Shrink(0).Clicked() {
						u.applyAlias(source, "", false)
					}
				})
			}
		})
	})
}

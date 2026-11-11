package onebot

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func contains(value, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(query)))
}
func (p *page) view(c *ui.Context) {
	p.env.Activity.Report("onebot", false, p.busy, nil)
	if p.settings != nil {
		p.settings.reportActivity()
	}
	if p.creation != nil {
		p.env.Activity.Report("onebot-create", p.creation.Dirty(), p.creation.busy, func() { p.creation = nil })
	} else {
		p.env.Activity.Report("onebot-create", false, false, nil)
	}
	p.takeIntents()
	ui.Row(c).Fill().MinWidth(0).AlignItems(ui.Stretch).Bind(&p.pageBox).Children(func() {
		p.sidebar(c)
		ui.Column(c).FillHeight().Grow(1).Basis(0).MinWidth(0).Bind(&p.contentBox).Children(func() {
			if p.loading {
				ui.Column(c).Fill().Center().Gap(12).Children(func() {
					kit.Loader(c)
					kit.Help(c, i18n.T("onebot.loading"))
				})
				return
			}
			if !p.loaded {
				kit.InlineError(c, p.error)
				if kit.Button(c, nt("retry"), icons.RefreshCw, p.refreshing).Clicked() {
					p.loadModels()
					p.refresh()
				}
				return
			}
			if p.section == "chat" && p.chat != nil {
				// Keep both real conversations usable when a topic opens in a
				// narrow content area. Resizing never closes either draft/controller.
				stacked := p.topicRoot != "" && p.contentBox.Bounds(c).W > 0 && p.contentBox.Bounds(c).W < 680
				layout := ui.Row(c)
				if stacked {
					layout = ui.Column(c)
				}
				layout.Fill().MinWidth(0).AlignItems(ui.Stretch).Children(func() {
					main := ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0)
					if stacked {
						main.FillWidth()
					} else {
						main.FillHeight()
					}
					main.Children(func() { p.chat.View(c) })
					p.topicView(c, stacked)
				})
				return
			}
			if p.section == "group" && p.group != nil {
				p.group.View(c)
				return
			}
			if p.section == "direct" && p.direct != nil {
				p.direct.View(c)
				return
			}
			ui.Column(c).Fill().MinWidth(0).Padding(16).Gap(12).Children(func() {
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					if kit.PrimaryButton(c, nt("create"), icons.Plus, p.busy).Clicked() {
						p.beginCreate()
					}
					if kit.Button(c, nt("market"), icons.Store, false).Clicked() {
						p.navigate("market")
						p.loadMarket()
					}
					if kit.Button(c, nt("import"), icons.Download, false).Clicked() {
						p.beginImport()
					}
					ui.Spacer(c)
					if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Disabled(p.refreshing).Clicked() {
						p.loadModels()
						p.refresh()
					}
				})
				kit.InlineError(c, p.error)
				if p.notice != "" {
					kit.Status(c, p.notice)
				}
				switch p.section {
				case "market":
					p.marketView(c)
				case "roster":
					p.rosterView(c, true)
				case "groups":
					p.groupListView(c)
				default:
					p.rosterView(c, false)
				}
			})
		})
	})
	if p.settings != nil {
		p.settings.View(c)
	}
	p.createView(c)
	p.importView(c)
	p.lifecycleView(c)
}
func (p *page) sidebar(c *ui.Context) {
	width := p.prefs.Sidebar
	if available := p.pageBox.Bounds(c).W; available > 0 {
		width = min(width, max(64, available-360))
	}
	collapsed := width < 120
	pal := kit.Colors(c)
	ui.Column(c).Width(width).Shrink(1).MinWidth(64).FillHeight().Padding(8).Gap(6).Background(pal.Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			if !collapsed {
				ui.Text(c, i18n.T("onebot.title")).FontWeight(500).Grow(1)
			}
			if kit.IconButton(c, icons.PanelLeftClose, nt("toggleSidebar")).Clicked() {
				if collapsed {
					p.prefs.Sidebar = 264
				} else {
					p.prefs.Sidebar = 64
				}
				p.savePreferences()
			}
		})
		for _, item := range []struct {
			id, label string
			icon      *ui.SVG
		}{{"home", nt("home"), icons.MessagesSquare}, {"roster", nt("roster"), icons.Users}, {"market", nt("market"), icons.Store}, {"groups", nt("groups"), icons.MessagesCircle}} {
			button := kit.GhostButton(c, "", item.icon, false).FillWidth().Tooltip(item.label).Label(item.label).MinWidth(0).Justify(ui.Start)
			if !collapsed {
				button.Children(func() { ui.Text(c, item.label).SingleLine().Ellipsis("…").MinWidth(0) })
			}
			if item.id == p.section {
				button.Background(pal.Selected)
			}
			if button.Clicked() {
				p.navigate(item.id)
				if item.id == "market" {
					p.loadMarket()
				}
			}
		}
		if !collapsed {
			kit.TextInput(c, &p.search).FillWidth().MinWidth(0).Placeholder(nt("search")).Label(nt("search"))
		}
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Gap(4).Children(func() {
			for _, bot := range p.sortedProfiles(false) {
				row := ui.ButtonBase(c.Key(bot.ID)).FillWidth().MinWidth(0).Padding(8).Gap(8).Role(ui.RoleButton).Label(rosterLabel(bot, p.profiles)).Tooltip(rosterLabel(bot, p.profiles))
				if p.botID == bot.ID && p.section == "chat" {
					row.Background(pal.Selected)
				} else if row.Hovered() {
					row.Background(c.Theme().SurfaceHover)
				}
				row.Children(func() {
					p.avatars.View(c, bot, 32)
					if !collapsed {
						ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
							ui.Text(c, rosterLabel(bot, p.profiles)).SingleLine().Ellipsis("…").MinWidth(0).FontWeight(500)
							preview := markdownPreview(bot.LastMessageText)
							if preview == "" {
								preview = bot.Description
							}
							kit.Help(c, preview).SingleLine().Ellipsis("…").MinWidth(0)
						})
					}
					if bot.UnreadCount > 0 {
						kit.Caption(c, nt("unread", "count", bot.UnreadCount)).Tooltip(nt("unread", "count", bot.UnreadCount))
					}
				})
				if row.Clicked() {
					p.openBot(bot.ID)
				}
				row.ContextMenu(func(m *ui.Menu) { p.botMenu(m, bot) })
			}
			for _, group := range p.groups {
				if kit.GhostButton(c, group.Name, icons.MessagesCircle, false).FillWidth().MinWidth(0).Tooltip(group.Name).Clicked() {
					p.openGroup(group.ID)
				}
			}
		})
		if kit.IconButton(c, icons.Settings2, nt("defaultModels")).Clicked() {
			p.openDefaultModels()
		}
		if p.eventError != "" {
			kit.InlineError(c, nt("eventsUnavailable"))
		}
	})
}
func (p *page) botMenu(m *ui.Menu, bot Profile) {
	if m.Item(nt("settings")).Chosen() {
		p.openSettings(bot.ID, "home")
	}
	if m.Item(bt("bots.groupChat.create.title")).Disabled(bot.Status != "" && bot.Status != "active").Chosen() {
		// Cindy's add-to-group: a new local group with this teammate already chosen.
		p.navigate("groups")
		editor := groupSettings(GroupSummary{ReplyMode: "all", SpeakingMode: "auto"})
		editor.members = []string{bot.ID}
		p.groupCreation = editor
	}
	if m.Item(nt("pin")).Chosen() {
		p.mutate("profiles.update", map[string]any{"input": map[string]any{"id": bot.ID, "pinned": bot.PinnedAt == nil}}, nil)
	}
	key := "hide"
	if bot.HiddenAt != nil {
		key = "show"
	}
	if m.Item(nt(key)).Chosen() {
		p.mutate("profiles.update", map[string]any{"input": map[string]any{"id": bot.ID, "hidden": bot.HiddenAt == nil}}, nil)
	}
	action := "pause"
	if bot.Status != "active" {
		action = "resume"
	}
	if m.Item(nt(action)).Disabled(bot.Status == "archived" || bot.Status == "deleting").Chosen() {
		p.lifecycle = &lifecycleDecision{bot: copyValue(bot), action: action, keepHistory: true}
	}
	if m.Item(nt("restart")).Disabled(bot.Status == "archived" || bot.Status == "deleting").Chosen() {
		p.lifecycle = &lifecycleDecision{bot: copyValue(bot), action: "restart", keepHistory: true}
	}
	if m.Item(nt("delete")).Chosen() {
		p.lifecycle = &lifecycleDecision{bot: copyValue(bot), action: "delete", keepHistory: true}
	}
}
func (p *page) rosterView(c *ui.Context, all bool) {
	rows := p.sortedProfiles(all)
	if len(rows) == 0 {
		ui.Column(c).Fill().Gap(12).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			kit.Help(c, nt("empty"))
			if kit.PrimaryButton(c, nt("create"), icons.Plus, false).Clicked() {
				p.beginCreate()
			}
		})
		return
	}
	ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Gap(8).Children(func() {
		for _, bot := range rows {
			ui.Row(c.Key(bot.ID)).FillWidth().MinWidth(0).Padding(12, 0).Gap(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				p.avatars.View(c, bot, 44)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					if kit.GhostButton(c, rosterLabel(bot, p.profiles), nil, false).MinWidth(0).Clicked() {
						p.openBot(bot.ID)
					}
					kit.Help(c, bot.Description).FillWidth()
					state := bot.Status
					if state == "" {
						state = "active"
					}
					kit.Caption(c, nt("status."+state))
				})
				kit.IconButton(c, icons.Ellipsis, nt("actions")).Menu(func(m *ui.Menu) { p.botMenu(m, bot) })
			})
		}
	})
}
func (p *page) lifecycleView(c *ui.Context) {
	d := p.lifecycle
	if d == nil {
		return
	}
	open := true
	deleting := d.action == "delete"
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nt(d.action), Message: nt("lifecycleHint", "name", d.bot.Name), Confirm: nt(d.action), Cancel: nt("cancel"), Danger: deleting, Busy: d.busy, ConfirmDisabled: deleting && d.confirmName != d.bot.Name, Error: d.error, Body: func() {
		if deleting {
			ui.Field(c, nt("confirmName", "name", d.bot.Name), func() { kit.TextInput(c, &d.confirmName).FillWidth().Label(nt("confirmName", "name", d.bot.Name)) })
			kit.Toggle(c, &d.keepHistory, nt("keepTaskHistory"), nt("keepTaskHistoryHint"))
			kit.Toggle(c, &d.retainWorktree, nt("retainWorktrees"), nt("retainWorktreesHint"))
		}
	}}) {
		d.busy = true
		input := map[string]any{"botId": d.bot.ID, "action": d.action}
		if deleting {
			input["confirmName"] = d.confirmName
			input["keepTaskHistory"] = d.keepHistory
			input["worktreeDisposition"] = "recycle"
			if d.retainWorktree {
				input["worktreeDisposition"] = "retain"
			}
		}
		oneBotCall(p, "lifecycle.action", input, func(result struct {
			Warnings []string `json:"warnings"`
		}, err error) {
			d.busy = false
			if err != nil {
				d.error = err.Error()
				return
			}
			p.lifecycle = nil
			p.notice = strings.Join(result.Warnings, "\n")
			if p.botID == d.bot.ID && deleting {
				p.closeContent()
				p.section = "roster"
			}
			p.refresh()
		})
	}
	if !open && !d.busy {
		p.lifecycle = nil
	}
}
func (p *page) takeIntents() {
	for _, intent := range p.env.TakeIntents("onebot") {
		var target struct {
			BotID     string `json:"botId"`
			SessionID string `json:"sessionId"`
			Page      string `json:"page"`
			// DirectThread opens a teammate-to-teammate thread (a trace card's link).
			DirectThread string `json:"directThread"`
		}
		if json.Unmarshal(intent.Options, &target) == nil && target.BotID != "" {
			if target.DirectThread != "" {
				p.openDirect(target.BotID, target.DirectThread)
				continue
			}
			if target.SessionID != "" {
				p.openSession(target.BotID, target.SessionID)
				// A companion interaction opens its conversation, not an unrelated
				// settings drawer on top of the question the user came to answer.
				if target.Page == "" {
					continue
				}
			}
			p.openSettings(target.BotID, target.Page)
		}
		if intent.ChatSettings {
			p.env.Open("chat", core.Intent{ChatSettings: true})
		}
	}
}

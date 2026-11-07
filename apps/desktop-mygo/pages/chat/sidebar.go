package chat

import (
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// sidebarView is the conversation list, as the React page's ChatSidebar:
// new chat, search, pinned conversations, automations, projects with
// their conversations, conversations outside projects, and settings and
// the archive at the bottom.
func (p *Chat) sidebarView(c *ui.Context) {
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c).Fill().Padding(12, 10).Gap(8).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(0, 0, 0, 8).Children(func() {
			ui.Text(c, i18n.T("chat.title")).FontWeight(500).Grow(1)
			if kit.IconButton(c, icons.PanelLeftClose, i18n.T("chat.hideSidebar")).Clicked() {
				p.setSidebar(false)
			}
		})
		if kit.GhostButton(c, i18n.T("chat.newChat"), icons.Plus, false).FillWidth().Justify(ui.Start).Disabled(p.busy).Clicked() {
			p.leaveAutomation(func() {
				p.run(func(done func(error)) {
					p.choose("", nil, func(err error) {
						if err == nil {
							p.newProjectID, p.newThinking, p.newFastMode = "", "", false
						}
						done(err)
					})
				})
			})
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(0, 8).Height(32).Radius(t.Radius).Border(1, pal.Hairline).Children(func() {
			ui.Icon(c, icons.Search).FontSize(15).TextColor(t.TextMuted).Shrink(0)
			ui.TextInputBase(c, &p.ui.search).Grow(1).MinWidth(0).Placeholder(i18n.T("chat.search")).Label(i18n.T("chat.search"))
		})
		visible := p.visibleConversations()
		ui.Scroll(c).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				var pinned []Conversation
				for _, conv := range visible {
					if conv.Pinned {
						pinned = append(pinned, conv)
					}
				}
				if len(pinned) > 0 {
					p.groupHeading(c, i18n.T("chat.pinned"), "")
					p.rows(c, pinned)
				}
				automations := kit.GhostButton(c, i18n.T("chat.automations"), icons.Timer, false).FillWidth().Justify(ui.Start).Disabled(p.busy)
				if p.ui.automations {
					automations.Background(pal.Selected)
				}
				if automations.Clicked() {
					p.env.RequestChange(func() { p.ui.details, p.ui.automations = false, true })
				}
				if kit.GhostButton(c, i18n.T("chat.openOrCreateProject"), icons.FolderPlus, false).FillWidth().Justify(ui.Start).Disabled(p.busy).Clicked() {
					p.openProjectDialog()
				}
				heading := i18n.T("chat.projects")
				if p.ui.archivedOnly() {
					heading = i18n.T("chat.archived")
				}
				all := len(p.catalog.Projects) > 0
				for _, pr := range p.catalog.Projects {
					all = all && slices.Contains(p.prefs.CollapsedProjects, pr.ID)
				}
				p.groupHeading(c, heading, "projects-all")
				for _, pr := range p.catalog.Projects {
					p.projectRow(c, pr, visible)
				}
				if !p.ui.archivedOnly() {
					p.groupHeading(c, i18n.T("chat.conversations"), "conversations")
				}
				if p.ui.archivedOnly() || !p.prefs.Sections["conversations"] || p.ui.search != "" {
					var loose []Conversation
					for _, conv := range visible {
						if conv.ProjectID == "" && !conv.Pinned {
							loose = append(loose, conv)
						}
					}
					p.rows(c, loose)
				}
				if p.ui.search != "" && len(visible) == 0 {
					kit.Help(c, i18n.T("chat.empty")).Padding(8)
				}
			})
		})
		if p.activityErr != "" {
			kit.InlineError(c, i18n.T("chat.sidebarStatusUnavailable")+" · "+p.activityErr)
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
			if kit.GhostButton(c, i18n.T("chat.settings"), icons.Settings2, false).Clicked() {
				p.openSettings()
			}
			ui.Spacer(c)
			label, icon := i18n.T("chat.showArchived"), icons.Archive
			if p.archived() {
				label, icon = i18n.T("chat.hideArchived"), icons.ArchiveRestore
			}
			b := kit.IconButton(c, icon, label).Selected(p.archived())
			if b.Clicked() {
				p.ui.archived = !p.ui.archived
			}
		})
	})
}

func (p *Chat) archived() bool { return p.ui.archived }

// archivedOnly reports whether the list shows archived conversations.
func (u *uiState) archivedOnly() bool { return u.archived }

// visibleConversations are the conversations the list shows: archived or
// not, matching the search, the latest first.
func (p *Chat) visibleConversations() []Conversation {
	query := strings.ToLower(p.ui.search)
	var out []Conversation
	for _, conv := range p.catalog.Conversations {
		if conv.Archived == p.archived() && strings.Contains(strings.ToLower(conv.Title+" "+conv.Directory), query) {
			out = append(out, conv)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

// groupHeading heads a section of the list; with a section key it folds
// the section (projects-all folds every project).
func (p *Chat) groupHeading(c *ui.Context, label, section string) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Margin(10, 0, 0).Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(t.TextMuted).Padding(7, 8).Grow(1)
		if section == "" || section == "conversations" && p.archived() {
			return
		}
		collapsed := p.prefs.Sections[section]
		if section == "projects-all" {
			collapsed = len(p.catalog.Projects) > 0
			for _, pr := range p.catalog.Projects {
				collapsed = collapsed && slices.Contains(p.prefs.CollapsedProjects, pr.ID)
			}
		}
		icon, key := icons.ChevronsDownUp, "chat.collapseSection"
		if collapsed {
			icon, key = icons.ChevronsUpDown, "chat.expandSection"
		}
		name := label
		if section == "projects-all" {
			key = "chat.collapseProjectConversations"
			if collapsed {
				key = "chat.expandProjectConversations"
			}
		}
		b := sidebarButton(c, icon, i18n.T(key, "name", name), 24, 24, 0.45, false)
		if section == "projects-all" && len(p.catalog.Projects) == 0 {
			b.Disabled(true)
		}
		if b.Clicked() {
			if section == "projects-all" {
				p.prefs.CollapsedProjects = nil
				if !collapsed {
					for _, pr := range p.catalog.Projects {
						p.prefs.CollapsedProjects = append(p.prefs.CollapsedProjects, pr.ID)
					}
				}
			} else {
				if p.prefs.Sections == nil {
					p.prefs.Sections = map[string]bool{}
				}
				p.prefs.Sections[section] = !collapsed
			}
			p.prefs.save()
		}
	})
}

// projectRow is a project, folding its conversations, with new chat and
// its menu.
func (p *Chat) projectRow(c *ui.Context, pr Project, visible []Conversation) {
	t := c.Theme()
	collapsed := slices.Contains(p.prefs.CollapsedProjects, pr.ID)
	ui.Column(c.Key("project-" + pr.ID)).FillWidth().Children(func() {
		heading := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8)
		hovered := heading.Hovered()
		heading.Children(func() {
			b := ui.ButtonBase(c).Grow(1).MinWidth(0).Padding(7, 2).Gap(6).Radius(t.Radius).Justify(ui.Start).AlignItems(ui.Center).
				Label(pr.Name).Expanded(!collapsed).Tooltip(pr.Path)
			b.Children(func() {
				chevron := icons.ChevronDown
				if collapsed {
					chevron = icons.ChevronRight
				}
				ui.Icon(c, chevron).FontSize(14).Shrink(0)
				ui.Icon(c, icons.FolderOpen).FontSize(15).Shrink(0)
				ui.Text(c, pr.Name).FontSize(sidebarFontSize).SingleLine().Ellipsis("…").MinWidth(0)
			})
			if b.Clicked() {
				if collapsed {
					p.prefs.CollapsedProjects = slices.DeleteFunc(p.prefs.CollapsedProjects, func(id string) bool { return id == pr.ID })
				} else {
					p.prefs.CollapsedProjects = append(p.prefs.CollapsedProjects, pr.ID)
				}
				p.prefs.save()
			}
			if sidebarButton(c, icons.Plus, i18n.T("chat.newChat")+" · "+pr.Name, 26, 25, 0.4, hovered).Disabled(p.busy).Clicked() {
				p.leaveAutomation(func() {
					id := pr.ID
					p.run(func(done func(error)) {
						p.choose("", nil, func(err error) {
							if err == nil {
								p.newProjectID, p.newThinking, p.newFastMode = id, "", false
							}
							done(err)
						})
					})
				})
			}
			more := sidebarButton(c, icons.Ellipsis, i18n.T("chat.moreFor", "name", pr.Name), 26, 25, 0.4, hovered).Disabled(p.busy)
			more.Menu(func(m *ui.Menu) {
				if m.Item(i18n.T("chat.renameProject")).Chosen() {
					p.ui.dialog = dialogState{kind: dialogRenameProject, project: pr, name: pr.Name}
				}
				if m.Item(i18n.T("chat.removeProject")).Chosen() {
					p.ui.dialog = dialogState{kind: dialogRemoveProject, project: pr}
				}
			})
		})
		if collapsed {
			return
		}
		var rows []Conversation
		for _, conv := range visible {
			if conv.ProjectID == pr.ID && !conv.Pinned {
				rows = append(rows, conv)
			}
		}
		p.rows(c, rows)
	})
}

// rows are conversation rows: the status in the gutter left of the title
// (the title's inset never moves), the title, a pin, and the menu.
func (p *Chat) rows(c *ui.Context, list []Conversation) {
	t, pal := c.Theme(), kit.Colors(c)
	for _, conv := range list {
		conv := conv
		activity := p.sidebarActivity(conv.ID)
		activity.Attention = activity.Attention && (activity.Phase == "error" || p.activeID != conv.ID)
		kind := statusKind(activity)
		if kind == "time" && p.titleJobs[conv.ID].pending {
			kind = "running"
		}
		active := p.activeID == conv.ID && !p.ui.automations
		ui.Column(c.Key("conversation-" + conv.ID)).FillWidth().Children(func() {
			item := ui.Row(c).FillWidth().AlignItems(ui.Center).Radius(7).Margin(1, 0)
			hovered := item.Hovered()
			switch {
			case active:
				item.Background(pal.Selected)
			case hovered:
				item.Background(t.SurfaceHover)
			}
			item.Children(func() {
				row := ui.ButtonBase(c).Grow(1).MinWidth(0).Padding(8, 10, 8, 28).Radius(7).Justify(ui.Start).AlignItems(ui.Center).Gap(8).
					FontSize(sidebarFontSize).LineHeight(1.4).Label(conv.Title).Selected(active).Disabled(p.busy)
				if active {
					// Selected also paints MyGo's accent background. This list
					// owns a neutral whole-row selection, so override BOTH.
					row.Background(ui.Transparent).TextColor(t.Text)
				}
				if kind == "time" {
					hint := i18n.T("chat.sidebarEmpty")
					if conv.HasMessages {
						hint = i18n.T("chat.sidebarIdle")
					}
					row.Tooltip(hint)
				}
				row.Children(func() {
					if kind != "time" {
						statusIcon(c, kind).Absolute().Left(4).Top(0).Bottom(0)
					}
					title := ui.Text(c, conv.Title).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
					if kind == "running" {
						title.FontWeight(500)
					}
					if conv.Pinned {
						ui.Icon(c, icons.Pin).FontSize(13).TextColor(t.TextMuted).Shrink(0)
					}
				})
				if row.Clicked() {
					p.leaveAutomation(func() {
						id := conv.ID
						p.run(func(done func(error)) { p.choose(id, nil, done) })
					})
				}
				p.conversationMenu(c, conv, false, hovered)
			})
			if job := p.titleJobs[conv.ID]; job.err != "" {
				kit.InlineError(c, i18n.T("chat.titleRequestFailed", "name", conv.Title)+" · "+job.err).Padding(0, 10, 4, 28)
			}
		})
	}
}

// statusIcon is a conversation's status in its row: running blue,
// errors and interruptions red, waiting for an answer orange (the
// user's chosen colors for this list), the rest muted.
func statusIcon(c *ui.Context, kind string) ui.Element {
	t := c.Theme()
	label := map[string]string{"running": "chat.sidebarRunning", "error": "chat.sidebarError", "awaiting": "chat.sidebarAwaiting",
		"paused": "chat.taskPaused", "interrupted": "chat.sidebarInterrupted", "done": "chat.sidebarDone"}[kind]
	box := ui.Box(c).Width(16).Justify(ui.Center).AlignItems(ui.Center).Label(i18n.T(label)).Tooltip(i18n.T(label))
	color := t.TextMuted
	switch kind {
	case "running":
		color = statusRunning(t)
	case "error", "interrupted":
		color = statusError(t)
	case "awaiting":
		color = statusAwaiting(t)
	}
	return box.Children(func() {
		if kind == "running" {
			ui.Spinner(c).Size(13, 13).TextColor(color)
			return
		}
		icon := map[string]*ui.SVG{"error": icons.CircleAlert, "awaiting": icons.MessageCircleQuestion, "paused": icons.Pause,
			"interrupted": icons.CircleStop, "done": icons.CircleCheck}[kind]
		ui.Icon(c, icon).FontSize(13).TextColor(color)
	})
}

// The status colors the user asked for in this list (blue, red, orange),
// muted to the window's restraint and lighter in the dark theme.
func statusRunning(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#6cb4ff")
	}
	return ui.Hex("#1f6fd1")
}

func statusError(t *ui.Theme) ui.Color { return t.Danger }

func statusAwaiting(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#f0a35c")
	}
	return ui.Hex("#c25e00")
}

// conversationMenu is a conversation's actions; the header's (open)
// adds those of the open conversation.
// In the list (hovered tells whether its row is) the trigger is the faint
// small one React's rows had; in the header (open) the ordinary button.
func (p *Chat) conversationMenu(c *ui.Context, conv Conversation, open, hovered bool) {
	running := open && p.view.Running
	label := i18n.T("chat.moreFor", "name", conv.Title)
	var more ui.Element
	if open {
		more = kit.IconButton(c, icons.Ellipsis, label)
	} else {
		more = sidebarButton(c, icons.Ellipsis, label, 27, 27, 0.35, hovered)
	}
	more.Disabled(p.busy && !open)
	more.Menu(func(m *ui.Menu) {
		if m.Item(i18n.T("chat.rename")).Disabled(p.busy).Chosen() {
			p.ui.dialog = dialogState{kind: dialogRename, conversation: conv, name: conv.Title}
		}
		if m.Item(i18n.T("chat.aiRename")).Disabled(p.titleJobs[conv.ID].pending || running).Chosen() {
			p.generateTitle(conv, nil)
		}
		pin := "chat.pin"
		if conv.Pinned {
			pin = "chat.unpin"
		}
		if m.Item(i18n.T(pin)).Chosen() {
			p.run(func(done func(error)) { p.updateConversation(conv, map[string]any{"pinned": !conv.Pinned}, done) })
		}
		archive := "chat.archive"
		if conv.Archived {
			archive = "chat.restore"
		}
		if m.Item(i18n.T(archive)).Disabled(running).Chosen() {
			p.run(func(done func(error)) { p.updateConversation(conv, map[string]any{"archived": !conv.Archived}, done) })
		}
		if open {
			empty := len(p.view.Messages) == 0
			if m.Item(i18n.T("chat.fork")).Disabled(running || empty).Chosen() {
				p.run(func(done func(error)) { p.fork("", done) })
			}
			if m.Item(i18n.T("chat.compact")).Disabled(running || empty).Chosen() {
				p.run(func(done func(error)) { p.simple("compact", map[string]any{"id": p.activeID}, done) })
			}
		}
		if m.Item(i18n.T("chat.sourcePath")).Chosen() {
			p.copy(conv.Directory)
		}
		if open && m.Item(i18n.T("chat.saveLocation")).Disabled(running || p.busy).Chosen() {
			p.openLocations()
		}
		if m.Item(i18n.T("chat.moveToProject")).Disabled(running || p.busy).Chosen() {
			p.err = ""
			target := conv
			p.ui.move = &target
		}
		if m.Item(i18n.T("chat.exportTask")).Disabled(p.busy).Chosen() {
			target := conv
			p.ui.exportOf, p.ui.exportState = &target, exportState{}
		}
		m.Separator()
		if m.Item(i18n.T("chat.delete")).Disabled(running || p.busy).Chosen() {
			p.ui.dialog = dialogState{kind: dialogDelete, conversation: conv}
		}
	})
}

// copy puts text on the clipboard.
func (p *Chat) copy(text string) { p.ui.services.WriteClipboard(text) }

// sidebarFontSize is the list's text size, React's 14px rows.
const sidebarFontSize = 14

// sidebarButton is a list row's small muted action, as React's: faint
// until its row is hovered, clear while hovered, focused or open.
func sidebarButton(c *ui.Context, icon *ui.SVG, label string, w, h, faint float32, rowHovered bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(w, h).Shrink(0).Radius(t.Radius).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	opacity := faint
	switch {
	case b.Hovered() || b.FocusVisible() || b.Pressed():
		opacity = 1
		b.Background(t.SurfaceHover)
	case rowHovered:
		opacity = 0.75
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted).Opacity(opacity) })
	return b
}

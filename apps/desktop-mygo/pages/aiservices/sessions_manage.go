package aiservices

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type sessionAgent struct {
	Agent, Name, Icon string
	Count             int
	Deletable         bool
}
type sessionTrash struct {
	Key, Agent, ID, Title, Cwd, Name string
	Deleted                          time.Time
	Size                             int64
	Items                            []struct{ From, Name string }
}
type sessionSearch struct {
	Hits            []struct{ ID, Text, Before, Match, After string }
	Searched, Total int
	Cut, Unread     bool
}
type sessionRelocation struct {
	From, To, WSL, Token, Backup string
	Sessions, Files              int
	Bytes                        int64
}
type sessionProviderMove struct{ ID, From, To, Backup string }

func (x *externalPanel) acceptSessions(raw json.RawMessage) {
	var state struct {
		Agents                        []sessionAgent
		Agent                         string
		Sessions                      []externalSession
		Trash                         []sessionTrash
		Terminal, Recording, Recorded bool
		TrashDir, Home                string
		Reads                         map[string]any
	}
	if json.Unmarshal(raw, &state) != nil || state.Sessions == nil || state.Agents == nil || state.Trash == nil {
		x.err = nativeT("invalidResponse")
		return
	}
	s := &x.sessions
	s.rows, s.agents, s.agent, s.trash = state.Sessions, state.Agents, state.Agent, state.Trash
	s.terminal, s.recording, s.recorded = state.Terminal, state.Recording, state.Recorded
	s.trashDir, s.home = state.TrashDir, state.Home
	if state.Reads != nil {
		s.reads = state.Reads
		s.chosen = map[string]bool{}
		for _, a := range rowsOf(state.Reads, "found") {
			s.chosen[textOf(a, "agent")] = boolOf(a, "on")
		}
	}
	if s.picked == nil {
		s.picked = map[string]bool{}
	}
	for id := range s.picked {
		if !slices.ContainsFunc(s.rows, func(r externalSession) bool { return r.ID == id && r.Deletable }) {
			delete(s.picked, id)
		}
	}
	if s.selected != nil {
		for _, row := range s.rows {
			if row.ID == s.selected.ID {
				s.selected = &row
				break
			}
		}
	}
}

// The management plugin keeps the upstream order: client tabs, search/select/
// trash, consent/recording, then folders and their sessions. Usage analytics
// remain in Usage; these rows act on the upstream management identifiers.
func (x *externalPanel) sessionsView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(18).Children(func() { x.sessionsContent(c) })
}

func (x *externalPanel) sessionsContent(c *ui.Context) {
	s := &x.sessions
	if s.reads == nil {
		return
	}
	if s.selected != nil {
		x.sessionDetail(c)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		for _, a := range s.agents {
			if kit.GhostButton(c, a.Name+" · "+kit.Itoa(a.Count), nil, false).Disabled(x.working).Clicked() {
				s.agent = a.Agent
				s.trashOn = false
				s.selecting = false
				clear(s.picked)
				s.search = nil
				x.readSessions()
			}
		}
		ui.Spacer(c).Grow(1)
		if kit.IconButton(c, icons.Settings, externalT("access")).Disabled(x.working).Clicked() {
			s.accessOpen = !s.accessOpen
		}
		if kit.Button(c, upstreamT("Trash")+" · "+kit.Itoa(len(s.trash)), icons.Trash2, false).Disabled(x.working).Clicked() {
			s.trashOn = !s.trashOn
			s.selecting = false
			clear(s.picked)
		}
	})
	if s.trashOn {
		x.sessionTrashView(c)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &s.query).Grow(1).MinWidth(180).Label(upstreamT("Search sessions")).Placeholder(upstreamT("Search sessions"))
		label := "Select"
		if s.selecting {
			label = "Done"
		}
		if kit.Button(c, upstreamT(label), nil, false).Disabled(x.working).Clicked() {
			s.selecting = !s.selecting
			if !s.selecting {
				clear(s.picked)
			}
		}
	})
	x.sessionSearchView(c)
	if !boolOf(s.reads, "asked") || s.accessOpen {
		x.sessionConsent(c)
	}
	x.sessionExportControl(c)
	if len(s.agents) == 0 {
		kit.Help(c, upstreamT("No sessions yet"))
		return
	}
	if s.recording || s.recorded || slices.ContainsFunc(s.rows, func(r externalSession) bool { return r.Gateway }) {
		x.sessionRecording(c)
	}
	if s.selecting {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, upstreamT("Select all"), nil, false).Disabled(x.working).Clicked() {
				for _, r := range s.rows {
					if r.Deletable {
						s.picked[r.ID] = true
					}
				}
			}
			if kit.Button(c, upstreamT("Select none"), nil, false).Disabled(x.working).Clicked() {
				clear(s.picked)
			}
			if kit.Button(c, upstreamT("Delete")+" · "+kit.Itoa(len(picked(s.picked))), icons.Trash2, false).Disabled(x.working || len(picked(s.picked)) == 0).Clicked() {
				x.deleteSessions(picked(s.picked))
			}
		})
	}
	rows := x.visibleManagedSessions()
	x.sessionProviderBar(c)
	folders := []string{}
	groups := map[string][]externalSession{}
	for _, r := range rows {
		key := r.Cwd
		if r.Gateway {
			key = upstreamT("Seen through the gateway")
		}
		if key == "" {
			key = upstreamT("Other sessions")
		}
		if _, ok := groups[key]; !ok {
			folders = append(folders, key)
		}
		groups[key] = append(groups[key], r)
	}
	for _, folder := range folders {
		children := groups[folder]
		holder := ui.Column(c.Key("folder:" + s.agent + ":" + folder)).FillWidth()
		open := ui.Local(holder, "open", func() bool { return true })
		holder.Children(func() {
			ui.Collapsible(c.Key(folder), folder+" · "+kit.Itoa(len(children)), open, func() {
				if s.agent == "claude" && !children[0].Gateway {
					if kit.Button(c, upstreamT("Move project"), icons.FolderOpen, false).Disabled(x.working).Clicked() {
						s.relocation = &sessionRelocation{From: children[0].Cwd, WSL: children[0].WSL}
					}
				}
				for _, r := range children {
					x.managedSessionRow(c.Key(r.Agent+"/"+r.ID), r)
				}
			})
		})
	}
	if len(rows) == 0 {
		kit.Help(c, externalT("empty"))
	}
	if s.relocation != nil {
		x.sessionRelocationView(c)
	}
	if len(s.providerMove) > 0 {
		x.sessionProviderDialog(c)
	}
}

func (x *externalPanel) sessionConsent(c *ui.Context) {
	s := &x.sessions
	kit.Section(c, nil, upstreamT("Show your agents' sessions?"), "", func() {
		for _, a := range rowsOf(s.reads, "found") {
			id := textOf(a, "agent")
			on := s.chosen[id]
			if ui.Checkbox(c.Key(id), &on, textOf(a, "name")).Disabled(x.working).Changed() {
				s.chosen[id] = on
			}
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.PrimaryButton(c, upstreamT("Allow selected"), nil, x.working).Disabled(x.working || len(picked(s.chosen)) == 0).Clicked() {
				x.sessionReads(false, s.chosen)
			}
			if kit.Button(c, upstreamT("Allow all"), nil, false).Disabled(x.working).Clicked() {
				x.sessionReads(true, map[string]bool{})
			}
			if boolOf(s.reads, "asked") && kit.Button(c, externalT("pause"), nil, false).Disabled(x.working).Clicked() {
				x.sessionReads(false, map[string]bool{})
			}
		})
	})
}

func (x *externalPanel) managedSessionRow(c *ui.Context, r externalSession) {
	s := &x.sessions
	ui.Column(c).FillWidth().Padding(10, 12).Gap(5).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if s.selecting {
				on := s.picked[r.ID]
				if ui.Checkbox(c, &on, r.Title).Disabled(x.working || !r.Deletable).Changed() {
					s.picked[r.ID] = on
				}
			} else {
				if kit.GhostButton(c, r.Title, nil, false).Clicked() {
					copy := r
					s.selected = &copy
					s.transcript = nil
				}
			}
			ui.Spacer(c).Grow(1)
			kit.Help(c, r.Last.Local().Format("2006-01-02 15:04"))
			if kit.IconButton(c, icons.Trash2, upstreamT("Delete")).Disabled(x.working || !r.Deletable).Tooltip(r.Undeletable).Clicked() {
				x.deleteSessions([]string{r.ID})
			}
		})
		kit.Help(c, fmt.Sprintf("%d %s · %d %s · $%.4f", r.Messages, upstreamT("messages"), r.Input+r.Output, upstreamT("tokens"), r.Cost))
		if s.search != nil {
			for _, hit := range s.search.Hits {
				if hit.ID == r.ID {
					kit.Help(c, hit.Before+hit.Match+hit.After)
					break
				}
			}
		}
		if r.Pending {
			kit.Status(c, i18n.T("capture.loading"))
		}
		if r.Provider != "" && r.UsesProvider != "" && r.Provider != r.UsesProvider {
			kit.Help(c, upstreamT("Provider")+": "+r.Provider+" → "+r.UsesProvider)
			if kit.Button(c, upstreamT("Move to current provider"), nil, false).Disabled(x.working).Clicked() {
				x.pickSessionProviders([]externalSession{r})
			}
		}
		if r.Resume != "" {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, externalT("copyCommand"), icons.Copy, false).Clicked() {
					c.Services().WriteClipboard(r.Resume)
				}
				if kit.Button(c, upstreamT("Continue in terminal"), nil, false).Disabled(x.working || !s.terminal).Clicked() {
					x.request("POST", "/api/sessions/terminal", map[string]string{"agent": r.Agent, "id": r.ID}, func(json.RawMessage) {})
				}
			})
		}
	})
}

func (x *externalPanel) deleteSessions(ids []string) {
	x.ask(upstreamT("Move to trash"), "/api/sessions/delete", map[string]any{"agent": x.sessions.agent, "ids": ids}, x.sessionMutationDone)
	names := []string{}
	for _, r := range x.sessions.rows {
		if slices.Contains(ids, r.ID) {
			names = append(names, r.Title)
		}
	}
	x.confirm.message = upstreamT("Move to trash") + "?\n" + strings.Join(names, "\n")
}

// A successful HTTP response may still contain individual refused operations.
// Keep those reasons visible and refresh the list without claiming full success.
func (x *externalPanel) sessionMutationDone(raw json.RawMessage) {
	var result struct {
		Refused           []struct{ ID, Key, Error string }
		Agent, ID, Backup string
		Moved             []sessionProviderMove
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		x.err = err.Error()
		return
	}
	lines := []string{}
	for _, r := range result.Refused {
		lines = append(lines, strings.TrimSpace(r.ID+" "+r.Key)+": "+r.Error)
	}
	x.message = strings.Join(lines, "\n")
	if len(lines) == 0 {
		x.message = externalT("saved")
	}
	if result.Backup != "" {
		x.message += "\n" + upstreamT("Backup") + ": " + result.Backup
	}
	if len(result.Moved) > 0 {
		x.sessions.lastMoves = result.Moved
	}
	x.sessions.selected = nil
	clear(x.sessions.picked)
	x.readSessions()
}

func (x *externalPanel) sessionTrashView(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(16).Children(func() { x.sessionTrashContent(c) })
}

func (x *externalPanel) sessionTrashContent(c *ui.Context) {
	s := &x.sessions
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		kit.Strong(c, upstreamT("Trash")).Grow(1)
		if kit.Button(c, upstreamT("Empty trash"), icons.Trash2, false).Disabled(x.working || len(s.trash) == 0).Clicked() {
			x.ask(upstreamT("Empty trash"), "/api/sessions/purge", map[string]bool{"all": true}, x.sessionMutationDone)
			x.confirm.message = upstreamT("Delete forever") + " · " + kit.Itoa(len(s.trash)) + "\n" + upstreamT("This cannot be undone.")
		}
	})
	if len(s.trash) == 0 {
		kit.Help(c, upstreamT("Trash is empty"))
	}
	for _, r := range s.trash {
		ui.Column(c.Key(r.Key)).FillWidth().Gap(6).Padding(10, 12).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			kit.Strong(c, r.Title)
			kit.Help(c, r.Name+" · "+r.Cwd+" · "+r.Deleted.Local().Format("2006-01-02 15:04"))
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, upstreamT("Restore"), nil, false).Disabled(x.working).Clicked() {
					x.request("POST", "/api/sessions/restore", map[string]string{"key": r.Key}, x.sessionMutationDone)
				}
				if kit.Button(c, upstreamT("Delete forever"), icons.Trash2, false).Disabled(x.working).Clicked() {
					x.ask(upstreamT("Delete forever"), "/api/sessions/purge", map[string]any{"keys": []string{r.Key}}, x.sessionMutationDone)
					x.confirm.message = r.Title + "\n" + upstreamT("This cannot be undone.")
				}
			})
		})
	}
}

func (x *externalPanel) sessionRecording(c *ui.Context) {
	s := &x.sessions
	kit.Section(c, nil, upstreamT("Record gateway conversations"), "", func() {
		on := s.recording
		if ui.Switch(c, &on).Label(upstreamT("Record gateway conversations")).Disabled(x.working).Changed() {
			x.ask(upstreamT("Record gateway conversations"), "/api/sessions/recording", map[string]bool{"on": on}, x.sessionMutationDone)
			x.confirm.message = upstreamT("Prompts, replies and tool results may contain private code and files. Store them on this server for up to 7 days (256 MiB total)? Recognized secrets are masked, but this is not a privacy guarantee.")
		}
		if s.recorded && kit.Button(c, upstreamT("Delete saved conversations…"), icons.Trash2, false).Disabled(x.working).Clicked() {
			x.ask(upstreamT("Stop recording and clear saved conversations"), "/api/sessions/recording", map[string]bool{"clear": true}, x.sessionMutationDone)
			x.confirm.message = upstreamT("Delete all locally recorded gateway conversation content? Usage totals and native session files are kept.")
		}
	})
}

func (x *externalPanel) sessionSearchView(c *ui.Context) {
	s := &x.sessions
	if s.queryShown != s.query {
		if s.searching {
			if x.cancel != nil {
				x.cancel()
				x.cancel = nil
			}
			x.generation++
			x.working = false
			s.searching = false
		}
		s.queryShown = s.query
		s.search = nil
		s.searchDue = c.Now().Add(300 * time.Millisecond)
	}
	if !s.searchDue.IsZero() {
		if c.Now().Before(s.searchDue) {
			c.After(s.searchDue.Sub(c.Now()))
		} else if !x.working {
			s.searchDue = time.Time{}
			if len([]rune(strings.TrimSpace(s.query))) >= 2 && s.agent != "" {
				s.searching = true
				x.request("GET", "/api/sessions/search?agent="+url.QueryEscape(s.agent)+"&q="+url.QueryEscape(s.query), nil, func(raw json.RawMessage) {
					s.searching = false
					var result sessionSearch
					if json.Unmarshal(raw, &result) != nil {
						x.err = nativeT("invalidResponse")
						return
					}
					s.search = &result
				})
			}
		}
	}
	if s.search != nil && (s.search.Cut || s.search.Unread) {
		kit.Help(c, fmt.Sprintf("%s: %d / %d", upstreamT("Searched"), s.search.Searched, s.search.Total))
	}
}
func (x *externalPanel) visibleManagedSessions() []externalSession {
	s := &x.sessions
	query := strings.ToLower(strings.TrimSpace(s.query))
	out := []externalSession{}
	for _, r := range s.rows {
		match := query == "" || strings.Contains(strings.ToLower(r.Title+" "+r.ID+" "+r.Cwd), query)
		if !match && s.search != nil {
			match = slices.ContainsFunc(s.search.Hits, func(h struct{ ID, Text, Before, Match, After string }) bool { return h.ID == r.ID })
		}
		if match {
			out = append(out, r)
		}
	}
	return out
}

func (x *externalPanel) sessionRelocationView(c *ui.Context) {
	r := x.sessions.relocation
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: upstreamT("Move project"), Cancel: i18n.T("common.cancel"), Width: 700, Busy: x.working, Error: x.err, Body: func() {
		kit.Help(c, r.From)
		previous := r.To
		field := ui.TextInput(c, &r.To).FillWidth().Label(upstreamT("New folder")).Disabled(x.working)
		if r.To != previous {
			r.Token = ""
		}
		widgets.DropArea(c, x.p.env, field, widgets.DropFolders, false, x.working, func(paths []string, err error) {
			if err != nil {
				x.err = err.Error()
			} else if len(paths) > 0 {
				r.To = paths[0]
				r.Token = ""
			}
		})
		if kit.Button(c, upstreamT("Choose a folder"), icons.FolderOpen, false).Disabled(x.working).Clicked() {
			core.Go(x.p.env, func() (string, error) { return x.p.env.Dialogs.PickFolder(upstreamT("New folder")) }, func(folder string, err error) {
				if err != nil {
					x.err = err.Error()
				} else if folder != "" {
					r.To = folder
					r.Token = ""
				}
			})
		}
		if r.Token != "" {
			kit.Help(c, fmt.Sprintf("%d %s · %d %s", r.Sessions, upstreamT("Sessions"), r.Files, upstreamT("Files")))
		}
	}, Actions: func() {
		if kit.Button(c, upstreamT("Preview"), nil, false).Disabled(x.working || r.To == "").Clicked() {
			x.request("POST", "/api/sessions/relocate-claude", map[string]string{"from": r.From, "to": r.To, "wsl": r.WSL}, func(raw json.RawMessage) {
				if json.Unmarshal(raw, r) != nil {
					x.err = nativeT("invalidResponse")
				}
			})
		}
		if kit.PrimaryButton(c, upstreamT("Move"), nil, false).Disabled(x.working || r.Token == "").Clicked() {
			x.request("POST", "/api/sessions/relocate-claude", map[string]string{"from": r.From, "to": r.To, "wsl": r.WSL, "token": r.Token}, func(raw json.RawMessage) { x.sessions.relocation = nil; x.sessionMutationDone(raw) })
		}
	}})
	if !open {
		x.sessions.relocation = nil
	}
}

func (x *externalPanel) sessionExportControl(c *ui.Context) {
	// External conversation export has its own explicit start/stop. Merely
	// saving the general collector preferences must never scan client files.
	s := &x.sessions
	if !s.accessOpen {
		return
	}
	label := externalT("startExport")
	if s.exporting {
		label = externalT("stopExport")
	}
	kit.Help(c, externalT("exportHint"))
	if kit.Button(c, label, nil, x.working).Disabled(x.working).Clicked() {
		x.working = true
		x.err = ""
		on := !s.exporting
		ticket := x.generation
		viewer := x.viewer
		core.Go(x.p.env, func() (bool, error) {
			return on, x.p.env.Host.CallCompletingInto("capture.ai.external.export", map[string]any{"viewer": viewer, "on": on}, nil)
		}, func(on bool, err error) {
			if ticket != x.generation {
				return
			}
			x.working = false
			if err != nil {
				x.err = err.Error()
				return
			}
			s.exporting = on
		})
	}
}

func (x *externalPanel) pickSessionProviders(rows []externalSession) {
	x.sessions.providerMove = rows
	x.sessions.movePicked = map[string]bool{}
	for _, r := range rows {
		x.sessions.movePicked[r.ID] = true
	}
}

func (x *externalPanel) sessionProviderBar(c *ui.Context) {
	s := &x.sessions
	if s.agent != "codex" {
		return
	}
	rows := []externalSession{}
	for _, r := range s.rows {
		if !r.ReadOnly && r.Provider != "" && r.UsesProvider != "" && r.Provider != r.UsesProvider {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 && len(s.lastMoves) == 0 {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if len(rows) > 0 {
			kit.Help(c, upstreamT("{n} Codex sessions were made with another provider ({from}): Codex lists only {to}'s sessions in its history now, so they don't show there.", "n", len(rows), "from", rows[0].Provider, "to", rows[0].UsesProvider)).Grow(1)
			if kit.Button(c, upstreamT("Move to {to}…", "to", rows[0].UsesProvider), nil, false).Disabled(x.working).Clicked() {
				x.pickSessionProviders(rows)
			}
		}
		if len(s.lastMoves) > 0 && kit.Button(c, upstreamT("Undo"), nil, false).Disabled(x.working).Clicked() {
			moves := []map[string]string{}
			for _, m := range s.lastMoves {
				moves = append(moves, map[string]string{"id": m.ID, "to": m.From})
			}
			x.request("POST", "/api/sessions/codex-provider", map[string]any{"moves": moves}, func(raw json.RawMessage) { x.sessionMutationDone(raw); s.lastMoves = nil })
		}
	})
}

func (x *externalPanel) sessionProviderDialog(c *ui.Context) {
	s := &x.sessions
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: upstreamT("Move sessions to {to}?", "to", s.providerMove[0].UsesProvider), Cancel: i18n.T("common.cancel"), Width: 760, Busy: x.working, Error: x.err, Body: func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, upstreamT("Select all"), nil, false).Clicked() {
				for _, r := range s.providerMove {
					s.movePicked[r.ID] = true
				}
			}
			if kit.Button(c, upstreamT("Deselect all"), nil, false).Clicked() {
				clear(s.movePicked)
			}
			if kit.Button(c, upstreamT("Invert selection"), nil, false).Clicked() {
				for _, r := range s.providerMove {
					s.movePicked[r.ID] = !s.movePicked[r.ID]
				}
			}
		})
		for _, r := range s.providerMove {
			on := s.movePicked[r.ID]
			if ui.Checkbox(c.Key(r.ID), &on, r.Title+" · "+r.Provider).Disabled(x.working).Changed() {
				s.movePicked[r.ID] = on
			}
		}
		kit.Help(c, upstreamT("Only the provider each one names is changed, in its files and in Codex's database: its messages, id, title and archive stay as they are. A copy of its files is kept in magpie's trash folder first, and Undo moves them back. A session written to in the last minute is left alone."))
	}, Actions: func() {
		if kit.PrimaryButton(c, upstreamT("Move"), nil, false).Disabled(x.working || len(picked(s.movePicked)) == 0).Clicked() {
			moves := []map[string]string{}
			for _, r := range s.providerMove {
				if s.movePicked[r.ID] {
					moves = append(moves, map[string]string{"id": r.ID, "to": r.UsesProvider})
				}
			}
			x.request("POST", "/api/sessions/codex-provider", map[string]any{"moves": moves}, func(raw json.RawMessage) { s.providerMove = nil; x.sessionMutationDone(raw) })
		}
	}})
	if !open {
		s.providerMove = nil
	}
}

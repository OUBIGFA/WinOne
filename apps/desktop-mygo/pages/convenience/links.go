package convenience

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/directorylinks"
)

// React source: features/directory-links.tsx and directory-links-menu.tsx.

// The links tab's own tabs.
const (
	linksBatch = iota
	linksScan
	linksMigration
)

// linkLaunchItem is a prefill waiting for the workspace: paths Explorer
// or a drop gave, for the sources or the destination, with options.
type linkLaunchItem struct {
	paths   []string
	options *linkLaunch
	target  bool
}

// linksTab is file and folder links: creating them, finding existing ones
// with their history, moving folders behind a link, and Explorer's menu.
// Every status shown comes from the Host's inspection of the entry's type,
// identity and access, never from an earlier successful creation.
type linksTab struct {
	env       *core.Env
	tab       int
	batch     *linkBatch
	scan      *linkScan
	archive   linkArchive
	migration *linkMigration
	records   []directorylinks.Record
	selected  map[string]bool
	busy      bool
	loading   bool
	closed    bool
	err       string
	notice    string
	action    int
	confirm   *directorylinks.Record
	forget    bool
	confirmOn bool
	queue     []linkLaunchItem
	open      map[string]bool
	details   map[string]bool
	menu      *directorylinks.ExplorerMenuState
	menuBusy  bool
	menuErr   string
}

func newLinksTab(env *core.Env) *linksTab {
	t := &linksTab{env: env, batch: newLinkBatch(env), scan: newLinkScan(env), migration: newLinkMigration(env),
		selected: map[string]bool{}, open: map[string]bool{}, details: map[string]bool{}, loading: true}
	t.archive.env = env
	// A prefill queued before this page showed.
	core.Go(env, func() (core.Intent, error) {
		var pending core.Intent
		err := env.Host.CallCompletingInto("directory-links.pending", nil, &pending)
		return pending, err
	}, func(pending core.Intent, err error) {
		if t.closed {
			return
		}
		if err != nil {
			t.err = err.Error()
			return
		}
		if pending.Requested {
			t.Queue(pending)
		}
	})
	core.Go(env, func() ([]directorylinks.Record, error) {
		var rows []directorylinks.Record
		if err := env.Host.CallCompletingInto("directory-links.list", nil, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}, func(rows []directorylinks.Record, err error) {
		if t.closed {
			return
		}
		if err != nil {
			t.loading, t.err = false, err.Error()
			return
		}
		t.records = rows
		var ids []string
		for _, row := range rows {
			if row.Status != "removed" {
				ids = append(ids, row.ID)
			}
		}
		if len(ids) == 0 {
			t.loading = false
			return
		}
		core.Go(env, func() ([]directorylinks.Record, error) {
			var checked []directorylinks.Record
			err := env.Host.CallCompletingInto("directory-links.inspectBatch", map[string]any{"ids": ids}, &checked)
			return checked, err
		}, func(checked []directorylinks.Record, err error) {
			t.loading = false
			if t.closed {
				return
			}
			if err != nil {
				t.err = err.Error()
				return
			}
			t.merge(checked)
		})
	})
	core.Go(env, func() (directorylinks.ExplorerMenuState, error) {
		var state directorylinks.ExplorerMenuState
		err := env.Host.CallCompletingInto("directory-links.menu.get", nil, &state)
		return state, err
	}, func(state directorylinks.ExplorerMenuState, err error) {
		if err != nil {
			t.menuErr = err.Error()
			return
		}
		t.menu = &state
	})
	return t
}

// Queue takes a prefill from Explorer's command; it applies once nothing
// on the page is busy.
func (t *linksTab) Queue(intent core.Intent) {
	item := linkLaunchItem{paths: intent.Paths}
	if len(intent.Options) > 0 && string(intent.Options) != "null" {
		var options linkLaunch
		if json.Unmarshal(intent.Options, &options) == nil {
			item.options = &options
		}
	}
	t.queue = append(t.queue, item)
}

// merge replaces records by ID and adds new ones.
func (t *linksTab) merge(changed []directorylinks.Record) {
	for _, row := range changed {
		if i := slices.IndexFunc(t.records, func(r directorylinks.Record) bool { return r.ID == row.ID }); i >= 0 {
			t.records[i] = row
		} else {
			t.records = append(t.records, row)
		}
	}
}

func (t *linksTab) childBusy() bool {
	return t.batch.busy || t.batch.source.loading || t.batch.target.loading || t.batch.rename != nil ||
		t.scan.busy || t.archive.busy || t.migration.busy() || t.menuBusy
}

// work runs one history operation at a time.
func (t *linksTab) work(run func() error, done func()) {
	if t.busy {
		return
	}
	t.action++
	id := t.action
	t.busy, t.err, t.notice = true, "", ""
	core.Do(t.env, run, func(err error) {
		if t.closed || id != t.action {
			return
		}
		t.busy = false
		if err != nil {
			t.err = err.Error()
			return
		}
		if done != nil {
			done()
		}
	})
}

func (t *linksTab) refresh(ids []string) {
	var checked []directorylinks.Record
	t.work(func() error {
		return t.env.Host.CallCompletingInto("directory-links.inspectBatch", map[string]any{"ids": ids}, &checked)
	}, func() { t.merge(checked) })
}

func (t *linksTab) forgetInvalid() {
	var result struct {
		Records   []directorylinks.Record `json:"records"`
		Forgotten int                     `json:"forgotten"`
	}
	t.work(func() error {
		return t.env.Host.CallCompletingInto("directory-links.forgetInvalid", map[string]any{"confirmed": true}, &result)
	}, func() {
		t.records = result.Records
		for id := range t.selected {
			if !slices.ContainsFunc(result.Records, func(r directorylinks.Record) bool { return r.ID == id }) {
				delete(t.selected, id)
			}
		}
		t.notice = i18n.T("links.invalidForgotten", "count", result.Forgotten)
	})
}

func (t *linksTab) confirmAction() {
	if t.confirm == nil {
		return
	}
	record, forget := *t.confirm, t.forget
	var row directorylinks.Record
	t.work(func() error {
		if forget {
			return t.env.Host.CallCompletingInto("directory-links.forget", map[string]any{"id": record.ID, "confirmed": true}, nil)
		}
		return t.env.Host.CallCompletingInto("directory-links.remove", map[string]any{"id": record.ID, "confirmed": true, "checkedAt": record.CheckedAt}, &row)
	}, func() {
		if forget {
			t.records = slices.DeleteFunc(t.records, func(r directorylinks.Record) bool { return r.ID == record.ID })
			t.notice = i18n.T("links.forgotten")
		} else {
			t.merge([]directorylinks.Record{row})
			t.notice = i18n.T("links.removed")
		}
		delete(t.selected, record.ID)
		t.confirm, t.confirmOn = nil, false
	})
}

func (t *linksTab) setMenu(enabled bool) {
	t.menuBusy, t.menuErr = true, ""
	core.Go(t.env, func() (directorylinks.ExplorerMenuState, error) {
		var state directorylinks.ExplorerMenuState
		err := t.env.Host.CallCompletingInto("directory-links.menu.set", map[string]any{"enabled": enabled}, &state)
		return state, err
	}, func(state directorylinks.ExplorerMenuState, err error) {
		t.menuBusy = false
		if err != nil {
			t.menuErr = err.Error()
			return
		}
		t.menu = &state
	})
}

// openMigration shows a history action in the migration tab, through the
// guard, as it replaces that tab's form.
func (t *linksTab) openMigration(record directorylinks.Record, action string, index int) {
	t.env.RequestChange(func() {
		t.migration.choose(migrationSelection{record: record, action: action, index: index})
		t.tab = linksMigration
	})
}

func (t *linksTab) Close() {
	t.closed = true
	t.migration.Close()
	t.batch.sourcePane.pasting = false
	for _, source := range []string{"directory-links", "directory-links-batch", "directory-links-scan", "directory-links-archive", "directory-links-migration", "directory-links-menu"} {
		t.env.Activity.Report(source, false, false, nil)
	}
}

// applyQueue takes waiting prefills once nothing runs: sources join the
// selection, the last destination opens, the last options apply.
func (t *linksTab) applyQueue(disabled bool) {
	if disabled || t.busy || t.childBusy() || len(t.queue) == 0 {
		return
	}
	var sources []string
	var destination *linkLaunchItem
	var options *linkLaunch
	for i := range t.queue {
		item := &t.queue[i]
		if item.target {
			destination = item
		} else {
			sources = append(sources, item.paths...)
		}
		if item.options != nil {
			options = item.options
		}
	}
	t.queue = nil
	if len(sources) > 0 {
		t.batch.add(sources)
	}
	if destination != nil {
		t.batch.dropDestination(destination.paths)
	}
	if options != nil {
		t.batch.launch(*options)
	}
	t.tab = linksBatch
}

func (t *linksTab) View(c *ui.Context, locked bool) {
	// Links act on the file system; they wait while the shell reads its
	// state, as the React page's actions did.
	disabled := false
	t.env.Activity.Report("directory-links", false, t.busy, nil)
	t.env.Activity.Report("directory-links-menu", false, t.menuBusy, nil)
	t.batch.Report()
	t.scan.Report()
	t.archive.Report()
	t.migration.Report()
	t.applyQueue(disabled)
	pageLocked := disabled || t.busy || t.childBusy() || t.loading
	changed := t.merge
	root := ui.Column(c).FillWidth().Gap(16)
	root.Children(func() {
		kit.SubTabs(c, &t.tab, []string{i18n.T("links.createWorkspace"), i18n.T("links.findExisting"), i18n.T("links.migrationTitle")}, t.env.RequestChange)
		if t.err != "" {
			kit.Notice(c, kit.Failure, "", t.err, nil)
		}
		if t.notice != "" {
			kit.Notice(c, kit.Info, "", t.notice, nil)
		}
		switch t.tab {
		case linksBatch:
			kit.Help(c, i18n.T("links.intro"))
			t.batch.View(c, disabled || t.loading || t.busy, changed)
		case linksScan:
			t.scan.View(c, disabled || t.loading || t.busy, changed)
			t.history(c, disabled, pageLocked)
		default:
			t.migration.View(c, disabled || t.busy || t.loading || t.batch.busy || t.scan.busy || t.archive.busy, changed)
		}
		t.menuView(c, disabled)
	})
	// Files dropped elsewhere on the page join the sources, as Explorer's
	// command does, and show the workspace.
	if files := root.DroppedFiles(); len(files) > 0 {
		t.queue = append(t.queue, linkLaunchItem{paths: append([]string(nil), files...)})
	}
	t.confirmDialog(c)
}

// menuView switches Explorer's "create link" command.
func (t *linksTab) menuView(c *ui.Context, disabled bool) {
	kit.Panel(c, func() {
		on := t.menu != nil && t.menu.Enabled
		unavailable := disabled || t.busy || t.menuBusy || t.menu == nil || t.menu.Conflict != ""
		if kit.Toggle(c, &on, i18n.T("links.explorerIntegration"), i18n.T("links.explorerMenuHint")).Disabled(unavailable).Changed() {
			t.setMenu(on)
		}
		if t.menu != nil && t.menu.Partial {
			ui.Column(c).Padding(0, 16, 12).Children(func() { kit.InlineError(c, i18n.T("links.explorerMenuPartial")) })
		}
		if t.menu != nil && t.menu.Conflict != "" {
			ui.Column(c).Padding(0, 16, 12).Children(func() { kit.InlineError(c, t.menu.Conflict) })
		}
		if t.menuErr != "" {
			ui.Column(c).Padding(0, 16, 12).Children(func() { kit.InlineError(c, t.menuErr) })
		}
	}).Label(i18n.T("links.explorerIntegration"))
}

// invalidLink reports a record whose link no longer stands. Unknown and
// offline results stay valid: failing to inspect proves nothing.
func invalidLink(r directorylinks.Record) bool {
	return slices.Contains([]string{"broken", "replaced", "target-changed", "source-changed", "unlinked", "removed"}, r.Status)
}

type linkGroup struct {
	key, target string
	directory   bool
	records     []directorylinks.Record
}

// groupLinks groups records by what they point at: folders first, then by
// path, numbers in order; newest first within a group.
func groupLinks(records []directorylinks.Record) []linkGroup {
	index := map[string]int{}
	var groups []linkGroup
	for _, record := range records {
		key := pathKey(record.TargetPath)
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, linkGroup{key: key, target: record.TargetPath, directory: record.ObjectType == "directory"})
		}
		groups[i].records = append(groups[i].records, record)
	}
	when := func(r directorylinks.Record) string {
		if r.CreatedAt != "" {
			return r.CreatedAt
		}
		return r.RegisteredAt
	}
	for i := range groups {
		slices.SortStableFunc(groups[i].records, func(a, b directorylinks.Record) int {
			if c := strings.Compare(when(b), when(a)); c != 0 {
				return c
			}
			return strings.Compare(a.LinkPath, b.LinkPath)
		})
	}
	slices.SortStableFunc(groups, func(a, b linkGroup) int {
		if a.directory != b.directory {
			if a.directory {
				return -1
			}
			return 1
		}
		return naturalCompare(a.key, b.key)
	})
	return groups
}

// naturalCompare orders text with its numbers by value, as
// localeCompare(..., {numeric: true}) does.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da > 0 && db > 0 {
			na, nb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func digits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

func formatLinkTime(value string) string {
	if value == "" {
		return i18n.T("links.neverChecked")
	}
	when, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return when.Local().Format("2006/1/2 15:04:05")
}

// history lists the registered links by what they point at, each with its
// latest inspection and actions.
func (t *linksTab) history(c *ui.Context, disabled, locked bool) {
	theme, pal := c.Theme(), kit.Colors(c)
	ui.Column(c).FillWidth().Gap(12).Role(ui.RoleGroup).Label(i18n.T("links.history")).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(10).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Children(func() {
				ui.Icon(c, icons.History).FontSize(18).TextColor(theme.TextMuted)
				kit.Heading(c, i18n.T("links.history")).Role(ui.RoleHeading).Level(3)
				kit.Caption(c, kit.Itoa(len(t.records)))
			})
			ui.Row(c).Gap(8).Wrap().Children(func() {
				var ids []string
				for id := range t.selected {
					ids = append(ids, id)
				}
				if kit.Button(c, i18n.T("links.checkSelected"), icons.RefreshCw, false).Disabled(locked || len(ids) == 0).Clicked() {
					t.refresh(ids)
				}
				if kit.Button(c, i18n.T("links.checkAll"), icons.RefreshCw, false).Disabled(locked || len(t.records) == 0).Clicked() {
					all := make([]string, len(t.records))
					for i, r := range t.records {
						all[i] = r.ID
					}
					t.refresh(all)
				}
				if kit.Button(c, i18n.T("links.forgetInvalid"), icons.Trash2, false).Tooltip(i18n.T("links.forgetInvalidHint")).
					Disabled(locked || len(t.records) == 0).Clicked() {
					t.forgetInvalid()
				}
			})
		})
		var ids []string
		for _, r := range t.records {
			if t.selected[r.ID] {
				ids = append(ids, r.ID)
			}
		}
		t.archive.View(c, ids, disabled || t.busy || t.loading || t.childBusy(), t.merge)
		if t.busy || t.loading {
			key := "links.working"
			if t.loading {
				key = "links.checking"
			}
			kit.Status(c, i18n.T(key))
		}
		if !t.loading && len(t.records) == 0 {
			kit.Help(c, i18n.T("links.empty"))
		}
		if len(t.records) > 0 {
			all := len(t.selected) == len(t.records)
			box := ui.Checkbox(c, &all, i18n.T("links.selectAll")).Disabled(locked)
			if len(t.selected) > 0 && !all {
				box.Mixed()
			}
			if box.Changed() {
				t.selected = map[string]bool{}
				if all {
					for _, r := range t.records {
						t.selected[r.ID] = true
					}
				}
			}
		}
		for _, group := range groupLinks(t.records) {
			open := t.open[group.key]
			ui.Column(c.Key(group.key)).FillWidth().Radius(kit.PanelRadius).Border(1, pal.Hairline).Children(func() {
				header := ui.ButtonBase(c).FillWidth().Padding(10, 12).Gap(8).Justify(ui.Start).AlignItems(ui.Center).Label(group.target).
					Tooltip(group.target).Expanded(open)
				if header.Hovered() {
					header.Background(theme.SurfaceHover)
				}
				header.Children(func() {
					chevron := icons.ChevronRight
					if open {
						chevron = icons.ChevronDown
					}
					ui.Icon(c, chevron).FontSize(18).TextColor(theme.TextMuted).Shrink(0)
					icon := icons.File
					if group.directory {
						icon = icons.Folder
					}
					ui.Icon(c, icon).FontSize(18).TextColor(theme.TextMuted).Shrink(0)
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, pathName(group.target)).SingleLine().Ellipsis("…").MinWidth(0)
						kit.Help(c, group.target).SingleLine().Ellipsis("…").MinWidth(0)
					})
					kit.Caption(c, i18n.T("links.groupCount", "count", len(group.records))).Shrink(0)
				})
				if header.Clicked() {
					t.open[group.key] = !open
				}
				if !open {
					return
				}
				ui.Column(c).FillWidth().Dividers(1, pal.Hairline).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
					for _, record := range group.records {
						t.recordView(c, record, locked)
					}
				})
			})
		}
	})
}

func (t *linksTab) recordView(c *ui.Context, record directorylinks.Record, locked bool) {
	theme := c.Theme()
	invalid := invalidLink(record)
	row := ui.Column(c.Key(record.ID)).FillWidth().Padding(12, 14).Gap(8)
	if invalid {
		row.TextColor(theme.TextMuted)
	}
	row.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			on := t.selected[record.ID]
			if ui.Checkbox(c, &on, "").Label(i18n.T("links.select", "name", record.Name)).Disabled(locked).Changed() {
				if on {
					t.selected[record.ID] = true
				} else {
					delete(t.selected, record.ID)
				}
			}
			kit.Strong(c, record.Name).Grow(1).MinWidth(0)
			kindLabel(c, record.Kind)
		})
		ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
			status := kit.Strong(c, i18n.T("links.status."+record.Status)).FontSize(kit.HelpSize(c))
			if record.Status != "active" {
				status.TextColor(theme.TextMuted)
			}
			reason := "links.reason." + record.Reason
			text := record.Reason
			if i18n.Has(reason) {
				text = i18n.T(reason)
			}
			kit.Help(c, text)
		})
		kit.Caption(c, i18n.T("links.checked")+"："+formatLinkTime(record.CheckedAt))
		ui.Row(c).FillWidth().GapX(12).GapY(6).Wrap().AlignItems(ui.Center).Children(func() {
			expected := "links.expectedTarget"
			if record.Kind == "hardlink" {
				expected = "links.recordedSource"
			}
			ui.Column(c).Basis(200).Grow(1).MinWidth(0).Gap(2).Children(func() {
				kit.Caption(c, i18n.T(expected))
				ui.Text(c, record.TargetPath).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			})
			ui.Icon(c, icons.ArrowRight).FontSize(18).TextColor(theme.TextMuted)
			ui.Column(c).Basis(200).Grow(1).MinWidth(0).Gap(2).Children(func() {
				kit.Caption(c, i18n.T("links.newEntry"))
				ui.Text(c, record.LinkPath).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			})
		})
		open := t.details[record.ID]
		ui.Collapsible(c, i18n.T("links.details"), &open, func() {
			ui.Column(c).FillWidth().Gap(6).Padding(6, 0).Children(func() {
				created, label := record.CreatedAt, "links.createdAt"
				if created == "" {
					created, label = record.RegisteredAt, "links.registeredAt"
				}
				facts := []kit.Fact{{Label: i18n.T(label), Value: formatLinkTime(created)}}
				if record.ActualType != "" {
					facts = append(facts, kit.Fact{Label: i18n.T("links.actualType"), Value: record.ActualType})
				}
				if record.ActualTarget != "" {
					facts = append(facts, kit.Fact{Label: i18n.T("links.rawTarget"), Value: record.ActualTarget})
				}
				if record.ResolvedTarget != "" {
					facts = append(facts, kit.Fact{Label: i18n.T("links.resolvedTarget"), Value: record.ResolvedTarget})
				}
				kit.Facts(c, facts)
				if record.Kind == "hardlink" && record.LinkCount > 0 {
					kit.Help(c, i18n.T("links.linksCount", "count", record.LinkCount))
				}
				if record.Accessible {
					kit.Help(c, i18n.T("links.dataAvailable"))
				}
				if record.Detail != "" {
					kit.InlineError(c, record.Detail)
				}
			})
		}).FontSize(kit.HelpSize(c))
		t.details[record.ID] = open
		if j := record.Migration; j != nil {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				phase := "links.migrationPhase." + j.Phase
				phaseText := j.Phase
				if i18n.Has(phase) {
					phaseText = i18n.T(phase)
				}
				kit.Help(c, i18n.T("links.migrationHistory")+" · "+phaseText+" · "+migrationSize(float64(j.Bytes)))
				if j.Message != "" {
					kit.InlineError(c, j.Message)
				}
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if j.Operation == "migrate" && j.Phase == "complete" && record.Status == "active" &&
						kit.Button(c, i18n.T("links.migrationRestore"), nil, false).Disabled(locked).Clicked() {
						t.openMigration(record, "restore", 0)
					}
					if j.Phase != "complete" && j.Phase != "failed" &&
						kit.Button(c, i18n.T("links.migrationAction.recover"), nil, false).Disabled(locked).Clicked() {
						t.openMigration(record, "recover", 0)
					}
				})
				for i, backup := range j.Retained {
					index := i
					ui.Column(c.Key(i)).FillWidth().Gap(4).Children(func() {
						kit.Help(c, i18n.T("links.migrationRetained")+"："+backup.Path).Selectable()
						ui.Row(c).Children(func() {
							if kit.Button(c, i18n.T("links.migrationAction.cleanup"), nil, false).Disabled(locked).Clicked() {
								t.openMigration(record, "cleanup", index)
							}
						})
					})
				}
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("links.check"), icons.RefreshCw, false).Disabled(locked || record.Status == "removed").Clicked() {
				t.refresh([]string{record.ID})
			}
			if kit.Button(c, i18n.T("links.openLocation"), icons.FolderOpen, false).Disabled(locked).Clicked() {
				id := record.ID
				t.work(func() error {
					return t.env.Host.CallCompletingInto("directory-links.reveal", map[string]any{"id": id, "side": "link"}, nil)
				}, nil)
			}
			migrating := record.Migration != nil && record.Migration.Phase != "complete"
			removable := !slices.Contains([]string{"removed", "restored", "replaced"}, record.Status) && !migrating && record.EntryIdentity.File != ""
			if kit.Button(c, i18n.T("links.remove"), icons.Trash2, false).Disabled(locked || !removable).Clicked() {
				r := record
				t.confirm, t.forget, t.confirmOn = &r, false, true
			}
			if kit.GhostButton(c, i18n.T("links.forget"), icons.Unlink, false).Disabled(locked).Clicked() {
				r := record
				t.confirm, t.forget, t.confirmOn = &r, true, true
			}
		})
	})
}

func (t *linksTab) confirmDialog(c *ui.Context) {
	if t.confirm == nil {
		return
	}
	record := *t.confirm
	title, hint, confirm := "links.removeTitle", "links.removeHint", "links.confirmRemove"
	if t.forget {
		title, hint, confirm = "links.forgetTitle", "links.forgetHint", "links.confirmForget"
	}
	if kit.Dialog(c, &t.confirmOn, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T(hint), Error: t.err, Danger: true, Busy: t.busy,
		Cancel: i18n.T("links.cancel"), Confirm: i18n.T(confirm),
		Body: func() {
			ui.Text(c, record.LinkPath).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			if !t.forget {
				target := record.ResolvedTarget
				if target == "" {
					target = record.ActualTarget
				}
				if target == "" {
					target = record.TargetPath
				}
				ui.Text(c, i18n.T("links.resolvedTarget")+"："+target).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			}
		},
	}) {
		t.confirmAction()
	}
	if !t.confirmOn && !t.busy {
		t.confirm = nil
	}
}

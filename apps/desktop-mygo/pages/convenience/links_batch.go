package convenience

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/directorylinks"
)

// React source: features/directory-links-batch.tsx.

// linkLaunch is what Explorer's command chose for the links it prefills.
type linkLaunch struct {
	Kind         string `json:"kind,omitempty"`
	Relative     *bool  `json:"relative,omitempty"`
	RetainScript *bool  `json:"retainScript,omitempty"`
	HideSuccess  *bool  `json:"hideSuccess,omitempty"`
}

// pathsFromText reads a path list: one per line, trimmed, quotes removed,
// each object once.
func pathsFromText(text string) []string {
	var paths []string
	seen := map[string]int{}
	for _, line := range splitLines(text) {
		line = strings.TrimSpace(line)
		if len(line) >= 2 && strings.HasPrefix(line, `"`) && strings.HasSuffix(line, `"`) {
			line = line[1 : len(line)-1]
		}
		if line == "" {
			continue
		}
		key := pathKey(line)
		if i, ok := seen[key]; ok {
			// The last spelling of a path wins, as a Map keeps it.
			paths[i] = line
			continue
		}
		seen[key] = len(paths)
		paths = append(paths, line)
	}
	return paths
}

// paneHeights keeps the workspace's chosen height in the profile, as the
// React window kept it in its storage.
const paneHeightFile = "directory-links-ui.json"

func readPaneHeight(directory string) float32 {
	var saved struct {
		PaneHeight float32 `json:"paneHeight"`
	}
	data, err := os.ReadFile(filepath.Join(directory, paneHeightFile))
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.PaneHeight < 256 || saved.PaneHeight > 4000 {
		return 0
	}
	return saved.PaneHeight
}

func writePaneHeight(directory string, height float32) {
	path := filepath.Join(directory, paneHeightFile)
	if height == 0 {
		_ = os.Remove(path)
		return
	}
	data, _ := json.Marshal(map[string]float32{"paneHeight": height})
	_ = os.WriteFile(path, data, 0o600)
}

type renaming struct {
	entry  directorylinks.BrowseEntry
	parent directorylinks.Identity
}

// linkBatch creates links: the chosen sources on the left, the
// destination's actual contents and the planned entries on the right, the
// type and Create below. Create previews and creates at once; the explicit
// preview stays available to review or export the plan.
type linkBatch struct {
	env      *core.Env
	text     string
	kind     string
	relative bool
	busy     bool
	err      string
	notice   string
	plan     *directorylinks.BatchPlan
	retain   bool
	hide     bool
	// custom is the single source's link name as typed; nil uses the
	// source's own name.
	custom     *string
	customFor  string
	moreOpen   bool
	rename     *renaming
	renameName string
	renameErr  string
	renameOpen bool
	source     directoryBrowser
	target     directoryBrowser
	split      kit.SplitState
	height     float32 // chosen height, 0 to fit the window
	shown      float32
	sourcePane folderPane
	targetPane folderPane
}

func newLinkBatch(env *core.Env) *linkBatch {
	b := &linkBatch{env: env, kind: "auto", split: kit.SplitState{Percent: 50}}
	b.source.env, b.target.env = env, env
	b.height = readPaneHeight(env.Directory)
	return b
}

func (b *linkBatch) sources() []string { return pathsFromText(b.text) }

func (b *linkBatch) resetPlan() { b.plan, b.notice, b.err = nil, "", "" }

// add selects paths as sources.
func (b *linkBatch) add(paths []string) {
	b.text = strings.Join(pathsFromText(strings.Join(append(b.sources(), paths...), "\n")), "\n")
	b.resetPlan()
}

func (b *linkBatch) pastePaths(text string) {
	paths := pathsFromText(text)
	if len(paths) == 0 {
		b.err = i18n.T("links.clipboardEmpty")
		return
	}
	b.add(paths)
}

func (b *linkBatch) selectPaths(paths []string, on bool) {
	if on {
		b.add(paths)
		return
	}
	keys := map[string]bool{}
	for _, path := range paths {
		keys[pathKey(path)] = true
	}
	b.text = strings.Join(slices.DeleteFunc(b.sources(), func(path string) bool { return keys[pathKey(path)] }), "\n")
	b.resetPlan()
}

// launch applies what Explorer's command chose.
func (b *linkBatch) launch(options linkLaunch) {
	if options.Kind != "" {
		b.kind = options.Kind
	}
	if options.Relative != nil {
		b.relative = *options.Relative
	}
	if options.RetainScript != nil {
		b.retain = *options.RetainScript
	}
	if options.HideSuccess != nil {
		b.hide = *options.HideSuccess
	}
	b.plan = nil
}

// work runs one operation at a time off the UI thread; done applies its
// result on it.
func (b *linkBatch) work(run func() error, done func()) {
	if b.busy {
		return
	}
	b.busy, b.err, b.notice = true, "", ""
	core.Do(b.env, run, func(err error) {
		b.busy = false
		if err != nil && !errors.Is(err, errCancelled) {
			b.err = err.Error()
			return
		}
		if err == nil && done != nil {
			done()
		}
	})
}

var errCancelled = errors.New("cancelled")

// dropDestination opens a dropped folder as the destination, after the
// Host read it: a file dropped by mistake keeps the folder chosen.
func (b *linkBatch) dropDestination(paths []string) {
	var listing directorylinks.DirectoryListing
	b.work(func() error {
		if len(paths) != 1 {
			return errors.New(i18n.T("links.dropOneFolder"))
		}
		if err := b.env.Host.CallCompletingInto("directory-links.browse", map[string]any{"directory": paths[0], "offset": 0}, &listing); err != nil {
			return errors.New(i18n.T("links.dropFolderOnly") + " " + err.Error())
		}
		return nil
	}, func() {
		b.target.accept(listing)
		b.resetPlan()
	})
}

// request is the preview's input, read on the UI thread.
type batchRequest struct {
	sources   []string
	directory string
	kind      string
	relative  bool
	linkName  string
	retain    bool
	hide      bool
}

func (b *linkBatch) request() (batchRequest, error) {
	sources := b.sources()
	r := batchRequest{sources: sources, directory: b.target.address, kind: b.kind, relative: b.relative, retain: b.retain, hide: b.hide}
	if len(sources) == 1 && b.custom != nil {
		if strings.TrimSpace(*b.custom) == "" {
			return r, errors.New(i18n.T("links.nameRequired"))
		}
		r.linkName = strings.TrimSpace(*b.custom)
	}
	return r, nil
}

// previewBatch reads the destination again and plans the batch there.
func (b *linkBatch) previewBatch(r batchRequest) (directorylinks.DirectoryListing, directorylinks.BatchPlan, error) {
	var listing directorylinks.DirectoryListing
	var plan directorylinks.BatchPlan
	if err := b.env.Host.CallCompletingInto("directory-links.browse", map[string]any{"directory": strings.TrimSpace(r.directory), "offset": 0}, &listing); err != nil {
		return listing, plan, err
	}
	params := map[string]any{"sources": r.sources, "directory": listing.Directory, "kind": r.kind, "relative": r.relative}
	if r.linkName != "" {
		params["linkName"] = r.linkName
	}
	err := b.env.Host.CallCompletingInto("directory-links.previewBatch", params, &plan)
	return listing, plan, err
}

func (b *linkBatch) preview() {
	r, err := b.request()
	if err != nil {
		b.err = err.Error()
		return
	}
	b.plan = nil
	var listing directorylinks.DirectoryListing
	var plan directorylinks.BatchPlan
	b.work(func() error {
		var err error
		listing, plan, err = b.previewBatch(r)
		return err
	}, func() {
		b.target.accept(listing)
		b.plan = &plan
	})
}

// create previews and creates the links; elevated creates them with
// administrator rights, which the user chose for this batch.
func (b *linkBatch) create(elevated bool, changed func([]directorylinks.Record)) {
	r, err := b.request()
	if err != nil {
		b.err = err.Error()
		return
	}
	if len(r.sources) == 0 || r.directory == "" {
		return
	}
	var listing directorylinks.DirectoryListing
	var result directorylinks.BatchResult
	retained := ""
	b.work(func() error {
		var plan directorylinks.BatchPlan
		var err error
		if listing, plan, err = b.previewBatch(r); err != nil {
			return err
		}
		if r.retain {
			destination, err := b.env.Dialogs.PickFolder(i18n.T("links.retainScript"))
			if err != nil {
				return err
			}
			if destination == "" {
				return errCancelled
			}
			var exported struct {
				Path string `json:"path"`
			}
			if err := b.env.Host.CallCompletingInto("directory-links.exportPlan", map[string]any{"token": plan.ID, "directory": destination}, &exported); err != nil {
				return err
			}
			retained = exported.Path
		}
		method := "directory-links.createBatch"
		if elevated {
			method += ".elevated"
		}
		if err := b.env.Host.CallCompletingInto(method, map[string]any{"token": plan.ID, "confirmed": elevated}, &result); err != nil {
			return err
		}
		// Keep both addresses; the new entries replace the planned ones.
		return b.env.Host.CallCompletingInto("directory-links.browse", map[string]any{"directory": listing.Directory, "offset": 0}, &listing)
	}, func() {
		changed(result.Records)
		if !r.hide || result.Completed != result.Total {
			b.notice = i18n.T("links.batchResult", "count", result.Completed, "total", result.Total)
			if retained != "" {
				b.notice += "\n" + i18n.T("links.exported", "path", retained)
			}
		}
		if result.Completed == result.Total {
			b.text = ""
		}
		b.plan = nil
		b.target.accept(listing)
	})
}

func (b *linkBatch) exportPlan() {
	if b.plan == nil {
		return
	}
	token := b.plan.ID
	path := ""
	b.work(func() error {
		destination, err := b.env.Dialogs.PickFolder(i18n.T("links.exportPlan"))
		if err != nil {
			return err
		}
		if destination == "" {
			return errCancelled
		}
		var exported struct {
			Path string `json:"path"`
		}
		err = b.env.Host.CallCompletingInto("directory-links.exportPlan", map[string]any{"token": token, "directory": destination}, &exported)
		path = exported.Path
		return err
	}, func() { b.notice = i18n.T("links.exported", "path", path) })
}

func (b *linkBatch) beginRename(entry directorylinks.BrowseEntry) {
	if b.busy || b.rename != nil || b.target.listing == nil {
		return
	}
	b.rename = &renaming{entry: entry, parent: b.target.listing.Identity}
	b.renameName, b.renameErr, b.renameOpen = entry.Name, "", true
}

// renameTarget renames an entry of the destination at once, apart from
// creating links; a new preview is needed afterwards.
func (b *linkBatch) renameTarget(disabled bool, changed func([]directorylinks.Record)) {
	if b.rename == nil || b.busy || disabled || strings.TrimSpace(b.renameName) == "" {
		return
	}
	request := directorylinks.RenameRequest{Path: b.rename.entry.Path, Name: b.renameName, EntryIdentity: b.rename.entry.Identity, ParentIdentity: b.rename.parent}
	old := b.rename.entry.Path
	b.busy, b.renameErr = true, ""
	core.Go(b.env, func() (directorylinks.RenameResult, error) {
		var result directorylinks.RenameResult
		err := b.env.Host.CallCompletingInto("directory-links.rename", request, &result)
		return result, err
	}, func(result directorylinks.RenameResult, err error) {
		b.busy = false
		if err != nil {
			b.renameErr = err.Error()
			return
		}
		b.resetPlan()
		changed(result.Records)
		b.rename, b.renameOpen = nil, false
		b.notice = i18n.T("links.renamed", "name", pathName(result.Path))
		if result.Warning != "" {
			b.err = result.Warning
		}
		sources := b.sources()
		for i, path := range sources {
			if pathKey(path) == pathKey(old) {
				sources[i] = result.Path
			}
		}
		b.text = strings.Join(sources, "\n")
		b.target.load(b.target.address, false, nil)
		if b.source.listing != nil {
			b.source.load(b.source.address, false, nil)
		}
	})
}

// Report records the workspace's activity: chosen sources count as an
// edit, as leaving would lose them.
func (b *linkBatch) Report() {
	dirty := b.text != "" || b.rename != nil && b.renameName != b.rename.entry.Name
	busy := b.busy || b.source.loading || b.target.loading || b.rename != nil
	b.env.Activity.Report("directory-links-batch", dirty, busy, func() {
		b.text = ""
		b.resetPlan()
	})
}

func (b *linkBatch) View(c *ui.Context, disabled bool, changed func([]directorylinks.Record)) {
	theme, pal := c.Theme(), kit.Colors(c)
	locked := disabled || b.busy || b.rename != nil
	sources := b.sources()
	single := ""
	if len(sources) == 1 {
		single = sources[0]
	}
	if single != b.customFor {
		b.customFor, b.custom = single, nil
	}
	selected := map[string]bool{}
	for _, path := range sources {
		selected[pathKey(path)] = true
	}
	listed := map[string]bool{}
	if b.source.listing != nil {
		for _, entry := range b.source.listing.Entries {
			listed[pathKey(entry.Path)] = true
		}
	}
	existing := map[string]bool{}
	kinds := map[string]string{}
	if b.target.listing != nil {
		for _, entry := range b.target.listing.Entries {
			existing[pathKey(entry.Name)] = true
			if entry.Kind != "" && entry.TargetPath != "" {
				kinds[pathKey(entry.TargetPath)] = entry.Kind
			}
		}
	}
	names := map[string]int{}
	for _, path := range sources {
		names[pathKey(pathName(path))]++
	}
	type previewRow struct {
		path, name string
		item       *directorylinks.Plan
		collision  bool
	}
	var rows []previewRow
	for _, path := range sources {
		row := previewRow{path: path, name: pathName(path)}
		if single != "" && b.custom != nil && strings.TrimSpace(*b.custom) != "" {
			row.name = strings.TrimSpace(*b.custom)
		}
		if b.plan != nil {
			for i := range b.plan.Items {
				if pathKey(b.plan.Items[i].Source) == pathKey(path) {
					row.item = &b.plan.Items[i]
				}
			}
		}
		row.collision = existing[pathKey(row.name)] || names[pathKey(row.name)] > 1
		rows = append(rows, row)
		switch {
		case row.item != nil:
			kinds[pathKey(path)] = row.item.Kind
		case b.kind != "auto":
			kinds[pathKey(path)] = b.kind
		default:
			kinds[pathKey(path)] = "pending"
		}
	}
	_, windowHeight := c.Size()
	height := b.height
	if height == 0 {
		height = max(256, windowHeight-400)
	}
	b.shown = height
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		if b.err != "" {
			kit.Notice(c, kit.Failure, "", b.err, nil)
		}
		if b.notice != "" {
			kit.Notice(c, kit.Info, "", b.notice, nil)
		}
		kit.Help(c, i18n.T("links.browserHint"))
		panes := ui.Column(c).FillWidth().Radius(kit.PanelRadius).Border(1, pal.Hairline).ClipX().ClipY()
		panes.Children(func() {
			kit.Split(c, &b.split, kit.SplitOptions{Label: i18n.T("links.resizePanes"), MinFirst: 30, MinSecond: 30}, func() {
				b.sourcePane = folderPane{side: "source", browser: &b.source, disabled: locked, selected: selected, kinds: kinds,
					onSelect: b.selectPaths, onPaste: b.pastePaths, onError: func(e string) { b.source.err = e },
					pasting: b.sourcePane.pasting,
					actions: func() { b.addMenu(c, locked) },
					extra: func() bool {
						var outside []string
						for _, path := range sources {
							if !listed[pathKey(path)] {
								outside = append(outside, path)
							}
						}
						if len(outside) == 0 {
							return false
						}
						ui.Column(c).FillWidth().Children(func() {
							ui.Row(c).Padding(10, 12, 4).Children(func() { kit.Caption(c, i18n.T("links.selectedOutside")) })
							for _, path := range outside {
								ui.Row(c.Key(pathKey(path))).FillWidth().Padding(10, 12).Gap(8).AlignItems(ui.Center).
									Background(theme.SurfaceHover).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
									on := true
									if ui.Checkbox(c, &on, "").Label(i18n.T("links.selectSource", "name", pathName(path))).Disabled(locked).Changed() {
										b.selectPaths([]string{path}, on)
									}
									ui.Icon(c, icons.Link).FontSize(16).TextColor(theme.TextMuted).Shrink(0)
									ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
										ui.Text(c, pathName(path)).MinWidth(0)
										kit.Help(c, path)
									})
								})
							}
						})
						return true
					}}
				pane := b.sourcePane.View(c, b.env, height)
				dropSources(c, b.env, pane, locked, b.add, func(e string) { b.err = e })
			}, func() {
				b.targetPane = folderPane{side: "target", browser: &b.target, disabled: locked, kinds: kinds,
					onAddress: b.resetPlan, onRename: b.beginRename, onError: func(e string) { b.target.err = e },
					extra: func() bool {
						if len(sources) == 0 || b.target.address == "" {
							return false
						}
						ui.Column(c).FillWidth().Role(ui.RoleList).Label(i18n.T("links.plannedContents")).Children(func() {
							for _, row := range rows {
								ui.Row(c.Key(pathKey(row.path))).FillWidth().Padding(10, 12).Gap(8).AlignItems(ui.Start).
									Background(theme.SurfaceHover).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Role(ui.RoleListItem).
									Children(func() {
										if row.collision {
											// A conflicting name is marked at its edge, as before.
											ui.Box(c).Width(3).FillHeight().Background(theme.Danger).Shrink(0)
										}
										ui.Icon(c, icons.Link).FontSize(16).TextColor(theme.TextMuted).Shrink(0)
										ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
											linkPath := destinationPath(b.target.address, row.name)
											if row.item != nil {
												linkPath = row.item.LinkPath
											}
											ui.Text(c, row.name).Tooltip(linkPath).MinWidth(0)
											kit.Help(c, "→ "+row.path).Tooltip(row.path)
										})
										if row.item != nil {
											kindLabel(c, row.item.Kind)
										}
										state, tone := "links.pendingCheck", theme.Text
										switch {
										case row.collision:
											state, tone = "links.nameCollision", theme.Danger
										case row.item != nil:
											state = "links.willCreate"
										}
										ui.Text(c, i18n.T(state)).FontSize(kit.HelpSize(c)).TextColor(tone).Shrink(0)
									})
							}
						})
						return true
					}}
				pane := b.targetPane.View(c, b.env, height)
				dropTarget(c, pane, locked, b.dropDestination)
			})
		})
		b.heightHandle(c, height)
		b.createBar(c, locked, single, sources, changed)
		if b.plan != nil {
			b.planView(c, locked)
		}
	})
	b.renameDialog(c, disabled, changed)
}

// heightHandle drags the panes' height; Up and Down step it, End takes
// the most, Home fits the window again.
func (b *linkBatch) heightHandle(c *ui.Context, height float32) {
	theme, pal := c.Theme(), kit.Colors(c)
	label := i18n.T("links.resizePaneHeight")
	handle := ui.Row(c).FillWidth().Height(14).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorResizeNS).
		Focusable().Role(ui.RoleSplitter).Label(label).Tooltip(label)
	set := func(value float32) {
		b.height = max(256, min(4000, value))
		writePaneHeight(b.env.Directory, b.height)
	}
	if _, dy, ok := handle.Dragged(); ok {
		set(height + dy)
	}
	switch {
	case handle.Shortcut(0, ui.KeyUp):
		set(height - 24)
	case handle.Shortcut(0, ui.KeyDown):
		set(height + 24)
	case handle.Shortcut(0, ui.KeyEnd):
		set(4000)
	case handle.Shortcut(0, ui.KeyHome):
		b.height = 0
		writePaneHeight(b.env.Directory, 0)
	}
	color := pal.Hairline
	if handle.Hovered() || handle.Pressed() {
		color = theme.Border
	}
	handle.Children(func() { ui.Icon(c, icons.GripHorizontal).FontSize(16).TextColor(color) })
}

// addMenu adds sources from pickers or the clipboard.
func (b *linkBatch) addMenu(c *ui.Context, locked bool) {
	button := kit.Button(c, i18n.T("links.addSources"), icons.Plus, false).Disabled(locked)
	button.Menu(func(m *ui.Menu) {
		if m.Item(i18n.T("links.pickSourceFiles")).Chosen() {
			var paths []string
			b.work(func() (err error) { paths, err = b.env.Dialogs.PickFiles(i18n.T("links.pickSourceFiles"), core.Filter{}); return err },
				func() { b.add(paths) })
		}
		if m.Item(i18n.T("links.pickSourceFolders")).Chosen() {
			var paths []string
			b.work(func() (err error) { paths, err = b.env.Dialogs.PickFolders(i18n.T("links.pickSourceFolders")); return err },
				func() { b.add(paths) })
		}
		if m.Item(i18n.T("links.pasteSources")).Chosen() {
			b.sourcePane.pasting = true
		}
	})
}

func (b *linkBatch) createBar(c *ui.Context, locked bool, single string, sources []string, changed func([]directorylinks.Record)) {
	theme, pal := c.Theme(), kit.Colors(c)
	ui.Row(c).FillWidth().Padding(16).GapX(20).GapY(14).Wrap().AlignItems(ui.End).Radius(kit.PanelRadius).
		Border(1, pal.Hairline).Background(theme.Surface).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			kit.Status(c, i18n.T("links.selectedCount", "count", len(sources)))
			if kit.GhostButton(c, i18n.T("links.clearSources"), nil, false).Disabled(locked || b.text == "").Clicked() {
				b.text = ""
				b.resetPlan()
			}
		})
		if single != "" {
			ui.Column(c).Gap(6).Basis(180).Grow(1).Children(func() {
				kit.Label(c, i18n.T("links.linkName"))
				if b.custom == nil {
					name := pathName(single)
					b.custom = &name
				}
				if ui.TextInput(c, b.custom).FillWidth().Label(i18n.T("links.linkName")).Disabled(locked).Changed() {
					b.resetPlan()
				}
			})
		}
		ui.Column(c).Gap(6).Basis(144).Grow(1).Children(func() {
			kit.Label(c, i18n.T("links.kind"))
			options := []kit.SelectOption{}
			for _, value := range []string{"auto", "hardlink", "junction", "symlink"} {
				options = append(options, kit.SelectOption{Value: value, Label: i18n.T("links." + value)})
			}
			changed, trigger := kit.Select(c, &b.kind, options)
			trigger.Label(i18n.T("links.kind")).Disabled(locked)
			if changed {
				if b.kind != "symlink" {
					b.relative = false
				}
				b.resetPlan()
			}
		})
		if b.kind == "symlink" && ui.Checkbox(c, &b.relative, i18n.T("links.relative")).Disabled(locked).Changed() {
			b.resetPlan()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(0, 0, 0, ui.Auto).Children(func() {
			unavailable := locked || len(sources) == 0 || b.target.address == ""
			if kit.PrimaryButton(c, i18n.T("links.createLinks"), icons.Link, b.busy).Disabled(unavailable).Clicked() {
				b.create(false, changed)
			}
			more := kit.Button(c, i18n.T("links.advancedOptions"), nil, false)
			if more.Clicked() {
				b.moreOpen = !b.moreOpen
			}
			ui.Popover(c, more, &b.moreOpen, func() {
				ui.Column(c).Width(352).MaxWidthPercent(90).Padding(12).Gap(10).Children(func() {
					if kit.Button(c, i18n.T("links.elevate"), icons.ShieldCheck, false).Disabled(unavailable).Clicked() {
						b.moreOpen = false
						b.create(true, changed)
					}
					ui.Checkbox(c, &b.retain, i18n.T("links.retainScript")).Disabled(locked)
					ui.Checkbox(c, &b.hide, i18n.T("links.hideSuccess")).Disabled(locked)
					if kit.GhostButton(c, i18n.T("links.preview"), nil, false).Disabled(unavailable).Clicked() {
						b.moreOpen = false
						b.preview()
					}
				})
			})
		})
	})
}

// planView is a previewed plan, to review or export before creating.
func (b *linkBatch) planView(c *ui.Context, locked bool) {
	plan := b.plan
	kit.Section(c, nil, i18n.T("links.batchPlan", "count", len(plan.Items)), "", func() {
		kit.Panel(c, func() {
			for _, item := range plan.Items {
				ui.Row(c.Key(item.ID)).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Start).Children(func() {
					kindLabel(c, item.Kind)
					kit.Facts(c, []kit.Fact{{Label: i18n.T("links.newEntry"), Value: item.LinkPath}, {Label: i18n.T("links.target"), Value: item.Source}})
				})
			}
		})
		kit.Help(c, i18n.T("links.direction"))
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("links.exportPlan"), icons.Download, false).Disabled(locked).Clicked() {
				b.exportPlan()
			}
		})
		kit.Help(c, i18n.T("links.elevationHint"))
	})
}

func (b *linkBatch) renameDialog(c *ui.Context, disabled bool, changed func([]directorylinks.Record)) {
	if b.rename == nil {
		return
	}
	title := "links.renameFile"
	if b.rename.entry.ObjectType == "directory" {
		title = "links.renameFolder"
	}
	open := b.renameOpen
	confirm := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T("links.renameHint"), Error: b.renameErr,
		Cancel: i18n.T("links.cancel"), Confirm: i18n.T("links.rename"), Busy: b.busy,
		ConfirmDisabled: disabled || strings.TrimSpace(b.renameName) == "" || b.renameName == b.rename.entry.Name,
		Body: func() {
			kit.Help(c, b.rename.entry.Path).Selectable()
			kit.Label(c, i18n.T("links.newName"))
			in := ui.TextInput(c, &b.renameName).FillWidth().Label(i18n.T("links.newName")).Disabled(b.busy).AutoFocus()
			if in.Changed() {
				b.renameErr = ""
			}
			if in.Submitted() {
				b.renameTarget(disabled, changed)
			}
		},
	})
	if confirm {
		b.renameTarget(disabled, changed)
	}
	if !open && !b.busy {
		b.rename, b.renameOpen = nil, false
	}
}

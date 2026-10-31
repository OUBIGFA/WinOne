package convenience

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/directorylinks"
)

// React source: features/directory-links-browser.tsx.

var drivePrefix = regexp.MustCompile(`^[A-Za-z]:\\?`)

// pathKey compares Windows paths as one object: separators, case, "." and
// ".." resolved, trailing separators dropped. The Host's answers may remove
// dot segments a user typed; the text shown stays as typed.
func pathKey(path string) string {
	value := strings.ReplaceAll(strings.TrimSpace(path), "/", `\`)
	prefix := ""
	switch {
	case strings.HasPrefix(value, `\\`):
		prefix = `\\`
	case drivePrefix.MatchString(value):
		prefix = drivePrefix.FindString(value)
	case strings.HasPrefix(value, `\`):
		prefix = `\`
	}
	var stack []string
	for _, part := range strings.Split(value[len(prefix):], `\`) {
		switch {
		case part == "" || part == ".":
		case part == ".." && len(stack) > 0 && stack[len(stack)-1] != "..":
			stack = stack[:len(stack)-1]
		default:
			stack = append(stack, part)
		}
	}
	return strings.ToLower(strings.TrimRight(prefix+strings.Join(stack, `\`), `\`))
}

// pathName is a path's last element.
func pathName(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func destinationPath(directory, name string) string {
	return strings.TrimRight(directory, `\/`) + `\` + name
}

// directoryBrowser reads one folder at a time. A read belongs to the
// address that asked for it: typing or navigating again discards older
// answers, so a slow folder never replaces the one shown.
type directoryBrowser struct {
	env      *core.Env
	address  string
	listing  *directorylinks.DirectoryListing
	loading  bool
	err      string
	sequence int
}

func (b *directoryBrowser) edit(value string) {
	b.sequence++
	b.address, b.listing, b.loading, b.err = value, nil, false, ""
}

// load reads directory, or the next page of the listing with more; done
// receives the listing once it shows.
func (b *directoryBrowser) load(directory string, more bool, done func(*directorylinks.DirectoryListing)) {
	requested := strings.TrimSpace(directory)
	if requested == "" {
		if done != nil {
			done(nil)
		}
		return
	}
	b.sequence++
	ticket := b.sequence
	var previous *directorylinks.DirectoryListing
	offset := 0
	if more && b.listing != nil {
		previous, offset = b.listing, len(b.listing.Entries)
	}
	b.address, b.loading, b.err = requested, true, ""
	if !more {
		b.listing = nil
	}
	core.Go(b.env, func() (directorylinks.DirectoryListing, error) {
		var value directorylinks.DirectoryListing
		err := b.env.Host.CallCompletingInto("directory-links.browse", map[string]any{"directory": requested, "offset": offset}, &value)
		return value, err
	}, func(value directorylinks.DirectoryListing, err error) {
		if ticket != b.sequence {
			return
		}
		b.loading = false
		if err != nil {
			b.err = err.Error()
			if done != nil {
				done(nil)
			}
			return
		}
		if previous != nil {
			value.Entries = append(append([]directorylinks.BrowseEntry(nil), previous.Entries...), value.Entries...)
		}
		b.listing, b.address = &value, value.Directory
		if done != nil {
			done(&value)
		}
	})
}

// accept shows a listing read elsewhere, as a dropped destination's.
func (b *directoryBrowser) accept(value directorylinks.DirectoryListing) {
	b.sequence++
	b.address, b.listing, b.loading, b.err = value.Directory, &value, false, ""
}

// folderPane is one side of the workspace: an address, the folder's
// entries, and what is planned there.
type folderPane struct {
	side     string // "source" or "target"
	browser  *directoryBrowser
	disabled bool
	// selected holds the chosen sources by pathKey; kinds tint rows by the
	// link type they have or will have.
	selected  map[string]bool
	kinds     map[string]string
	onSelect  func(paths []string, selected bool)
	onAddress func()
	onRename  func(entry directorylinks.BrowseEntry)
	onPaste   func(text string)
	onError   func(string)
	actions   func()
	// extra shows what is planned or chosen beside the listing, and
	// reports whether it showed anything.
	extra func() bool
	pasting   bool
}

func (p *folderPane) navigate(path string) {
	if p.onAddress != nil {
		p.onAddress()
	}
	p.browser.load(path, false, nil)
}

// kindLabel is a link type's badge.
func kindLabel(c *ui.Context, kind string) {
	theme, pal := c.Theme(), kit.Colors(c)
	ui.Box(c).Padding(1, 7).Radius(6).Border(1, pal.Hairline).Background(theme.SurfaceHover).Shrink(0).Children(func() {
		ui.Text(c, i18n.T("links."+kind)).FontSize(kit.CaptionSize(c)).FontWeight(500).TextColor(theme.TextMuted).NoWrap()
	})
}

func (p *folderPane) View(c *ui.Context, env *core.Env, height float32) ui.Element {
	theme, pal := c.Theme(), kit.Colors(c)
	source := p.side == "source"
	title, addressLabel := "links.targetFolder", "links.location"
	if source {
		title, addressLabel = "links.sourceFolder", "links.sourceFolder"
	}
	b := p.browser
	pane := ui.Column(c).FillWidth().Height(height).MinHeight(0).Role(ui.RoleGroup).Label(i18n.T(title))
	pane.Children(func() {
		ui.Column(c).FillWidth().Padding(12).Gap(10).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
			kit.Strong(c, i18n.T(title))
			address := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center)
			address.Children(func() {
				in := ui.TextInput(c, &b.address).Grow(1).MinWidth(0).Placeholder(i18n.T("links.enterFolder")).
					Label(i18n.T(addressLabel)).Disabled(p.disabled)
				if in.Changed() {
					b.edit(b.address)
					if p.onAddress != nil {
						p.onAddress()
					}
				}
				if in.Submitted() {
					p.navigate(b.address)
				}
				open := "links.openTargetFolder"
				if source {
					open = "links.openSourceFolder"
				}
				if kit.OutlineIconButton(c, icons.ChevronRight, i18n.T(open), false).Disabled(p.disabled || strings.TrimSpace(b.address) == "").Clicked() {
					p.navigate(b.address)
				}
			})
			// A folder dropped on the address opens there.
			widgets.DropArea(c, env, address, widgets.DropFolders, false, p.disabled, func(paths []string, err error) {
				if err != nil {
					p.onError(err.Error())
					return
				}
				p.navigate(paths[0])
			})
			ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
				browse := "links.pickLocation"
				if source {
					browse = "links.browseSource"
				}
				if kit.Button(c, i18n.T("links.browseFolder"), icons.FolderOpen, false).Label(i18n.T(browse)).Disabled(p.disabled).Clicked() {
					core.Go(env, func() (string, error) { return env.Dialogs.PickFolder(i18n.T("links.browseFolder")) }, func(path string, err error) {
						if err != nil {
							p.onError(err.Error())
						} else if path != "" {
							p.navigate(path)
						}
					})
				}
				parent, refresh := "links.targetParent", "links.refreshTarget"
				if source {
					parent, refresh = "links.sourceParent", "links.refreshSource"
				}
				if kit.OutlineIconButton(c, icons.ArrowUp, i18n.T(parent), false).Tooltip(i18n.T("links.parentFolder")).
					Disabled(p.disabled || b.listing == nil || b.listing.Parent == "").Clicked() {
					p.navigate(b.listing.Parent)
				}
				if kit.OutlineIconButton(c, icons.RefreshCw, i18n.T(refresh), false).Tooltip(i18n.T("links.refreshFolder")).
					Disabled(p.disabled || b.address == "" || b.loading).Clicked() {
					p.navigate(b.address)
				}
				if p.actions != nil {
					p.actions()
				}
			})
		})
		if b.err != "" {
			ui.Column(c).Padding(8, 12, 0).Shrink(0).Children(func() { kit.Notice(c, kit.Failure, "", b.err, nil) })
		}
		var entries []directorylinks.BrowseEntry
		if b.listing != nil {
			entries = b.listing.Entries
		}
		var selectable []directorylinks.BrowseEntry
		for _, entry := range entries {
			if entry.Error == "" {
				selectable = append(selectable, entry)
			}
		}
		if source && len(selectable) > 0 {
			chosen := 0
			for _, entry := range selectable {
				if p.selected[pathKey(entry.Path)] {
					chosen++
				}
			}
			all := chosen == len(selectable)
			ui.Row(c).FillWidth().Padding(10, 12).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
				box := ui.Checkbox(c, &all, i18n.T("links.selectCurrentFolder")).Disabled(p.disabled || b.loading)
				if chosen > 0 && chosen < len(selectable) {
					box.Mixed()
				}
				if box.Changed() {
					paths := make([]string, len(selectable))
					for i, entry := range selectable {
						paths[i] = entry.Path
					}
					p.onSelect(paths, all)
				}
			})
		}
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Role(ui.RoleList).Children(func() {
			shown := false
			if p.extra != nil {
				shown = p.extra()
			}
			switch {
			case b.loading && b.listing == nil:
				ui.Column(c).Padding(20, 16).Children(func() { kit.Status(c, i18n.T("links.loadingFolder")) })
			case !b.loading && b.listing == nil && b.err == "" && !shown:
				empty := "links.targetEmpty"
				if source {
					empty = "links.sourceEmpty"
				}
				ui.Column(c).Padding(20, 16).Children(func() { kit.Help(c, i18n.T(empty)) })
			case b.listing != nil && len(entries) == 0:
				ui.Column(c).Padding(20, 16).Children(func() { kit.Help(c, i18n.T("links.folderEmpty")) })
			}
			for _, entry := range entries {
				p.entryRow(c, entry, theme, pal)
			}
			if b.listing != nil && b.listing.HasMore {
				ui.Row(c).Padding(12).Children(func() {
					label := i18n.T("links.loadMore", "shown", len(entries), "total", b.listing.Total)
					if kit.Button(c, label, nil, b.loading).Disabled(p.disabled).Clicked() {
						b.load(b.address, true, nil)
					}
				})
			}
		})
	})
	// Pasting a list of paths in the source pane, outside its address,
	// selects them; the clipboard is read with the next frame.
	if source && p.onPaste != nil {
		if p.pasting {
			p.pasting = false
			if text := c.ReadClipboard(); text != "" {
				p.onPaste(text)
			}
		}
		pane.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputCommand && ev.Text == "paste" && !p.disabled {
				p.pasting = true
				return true
			}
			return false
		})
	}
	return pane
}

func (p *folderPane) entryRow(c *ui.Context, entry directorylinks.BrowseEntry, theme *ui.Theme, pal kit.Palette) {
	folder := entry.ObjectType == "directory"
	key := pathKey(entry.Path)
	selected := p.selected[key]
	kind := entry.Kind
	if tint := p.kinds[key]; tint != "" {
		kind = tint
	}
	row := ui.Row(c.Key(entry.Path)).FillWidth().Padding(10, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
		BorderColor(pal.Hairline).Role(ui.RoleListItem)
	if selected || kind != "" {
		row.Background(theme.SurfaceHover)
	}
	canRename := p.onRename != nil && !p.disabled && entry.Identity.File != ""
	if p.onRename != nil {
		row.Focusable()
		if row.Shortcut(0, ui.KeyF2) && canRename {
			p.onRename(entry)
		}
	}
	row.Children(func() {
		if p.side == "source" {
			on := selected
			if ui.Checkbox(c, &on, "").Label(i18n.T("links.selectSource", "name", entry.Name)).Disabled(p.disabled || entry.Error != "").Changed() {
				p.onSelect([]string{entry.Path}, on)
			}
		}
		icon := icons.File
		if folder {
			icon = icons.Folder
		}
		ui.Icon(c, icon).FontSize(16).TextColor(theme.TextMuted).Shrink(0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			if folder {
				open := kit.GhostButton(c, entry.Name, nil, false).Label(i18n.T("links.openFolder", "name", entry.Name)).
					Tooltip(entry.Path).Disabled(p.disabled)
				if open.Clicked() {
					p.navigate(entry.Path)
				}
			} else {
				ui.Text(c, entry.Name).Tooltip(entry.Path).MinWidth(0)
			}
			if entry.TargetPath != "" {
				kit.Help(c, "→ "+entry.TargetPath).Tooltip(entry.TargetPath)
			}
			if entry.Error != "" {
				kit.InlineError(c, entry.Error)
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			switch {
			case entry.Kind != "":
				kindLabel(c, entry.Kind)
				if entry.Error == "" {
					kit.Caption(c, i18n.T("links.linked"))
				}
			case folder:
				kit.Caption(c, i18n.T("links.directoryType"))
			default:
				kit.Caption(c, i18n.T("links.fileType"))
			}
			if p.onRename != nil {
				label := i18n.T("links.renameEntry", "name", entry.Name)
				if kit.IconButton(c, icons.PencilLine, label+" (F2)").Label(label).Disabled(!canRename).Clicked() {
					p.onRename(entry)
				}
			}
		})
	})
}

package chat

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogRename
	dialogDelete
	dialogRenameProject
	dialogRemoveProject
	dialogProject
	dialogLocations
)

// dialogState is the open dialog of the conversation list and header.
type dialogState struct {
	kind         dialogKind
	conversation Conversation
	project      Project
	name         string
	// The project dialog: open a folder, or create one inside it.
	create     bool
	folder     string
	permission model.Permission
	// The locations dialog's working and save folders.
	working, saved string
	open           bool
}

// exportState is the export dialog's form.
type exportState struct {
	encrypt, show, excludeMedia bool
	password, confirmation      string
	working                     bool
	err                         string
	oversize                    *exportOutcome
}

func (p *Chat) openProjectDialog() {
	p.err = ""
	p.ui.dialog = dialogState{kind: dialogProject, permission: model.PermissionAsk, open: true}
}

func (p *Chat) openLocations() {
	working := p.location
	if c := p.conversation(); c != nil {
		working = c.Directory
	} else if pr := p.catalog.project(p.newProjectID); pr != nil {
		working = pr.Path
	}
	saved := p.saveLocation
	if c := p.conversation(); c != nil {
		saved = c.SessionDirectory
	}
	p.ui.dialog = dialogState{kind: dialogLocations, working: working, saved: saved, open: true}
}

// pickFolder chooses a folder, then gives it to use on the UI thread.
func (p *Chat) pickFolder(use func(string)) {
	core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(i18n.T("chat.chooseLocation")) },
		func(path string, err error) {
			if err != nil {
				p.err = err.Error()
				return
			}
			if path != "" {
				use(path)
			}
		})
}

// dropFolder lets a folder field take a folder dragged from File Explorer,
// as choosing it would.
func (p *Chat) dropFolder(c *ui.Context, zone ui.Element, disabled bool, use func(string)) {
	widgets.DropArea(c, p.env, zone, widgets.DropFolders, false, disabled, func(paths []string, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		use(paths[0])
	})
}

// dialogs draws the page's open dialog.
func (p *Chat) dialogs(c *ui.Context) {
	p.workdirPluginsDialog(c)
	d := &p.ui.dialog
	if d.kind != dialogNone {
		d.open = true
		p.mainDialog(c, d)
		if !d.open && !p.busy {
			*d = dialogState{}
		} else if !d.open {
			d.open = true
		}
	}
	if p.ui.move != nil {
		p.moveDialog(c)
	}
	if p.ui.exportOf != nil {
		p.exportDialog(c)
	}
	if p.queuedSend != nil {
		// While a turn runs, a new message is queued after it or steered
		// into it; Keep editing returns to the draft.
		open := true
		text := p.queuedSend.text
		steer := kit.Dialog(c, &open, kit.DialogOptions{Title: i18n.T("chat.confirmFollowup"), Message: i18n.T("chat.confirmFollowupHint"),
			Cancel: i18n.T("chat.keepEditing"), Confirm: i18n.T("chat.steerSend"),
			Body: func() {
				if text != "" {
					kit.Help(c, text).MaxHeight(160)
				}
			},
			Actions: func() {
				if kit.Button(c, i18n.T("chat.queueSend"), nil, false).Clicked() {
					p.confirmSend(model.DeliveryQueue)
				}
			}})
		switch {
		case steer:
			p.confirmSend(model.DeliverySteer)
		case !open:
			p.queuedSend = nil
		}
	}
	if p.ui.confirmFull {
		if kit.Dialog(c, &p.ui.confirmFull, kit.DialogOptions{Title: i18n.T("chat.fullTitle"), Message: i18n.T("chat.fullDescription"),
			Cancel: i18n.T("chat.cancel"), Confirm: i18n.T("chat.enableFull"), Danger: true}) {
			p.ui.confirmFull = false
			p.run(func(done func(error)) { p.configure(nil, model.PermissionBypass, done) })
		}
	}
}

// mainDialog is rename, delete, project, project rename/removal and the
// locations dialog.
func (p *Chat) mainDialog(c *ui.Context, d *dialogState) {
	o := kit.DialogOptions{Cancel: i18n.T("chat.cancel"), Busy: p.busy, Error: p.err, Width: 460}
	confirm := func(done func(error)) {}
	switch d.kind {
	case dialogRename:
		o.Title, o.Confirm = i18n.T("chat.rename"), i18n.T("chat.rename")
		o.ConfirmDisabled = strings.TrimSpace(d.name) == ""
		o.Body = func() {
			kit.TextInput(c, &d.name).FillWidth().Label(i18n.T("chat.newName")).AutoFocus()
		}
		target := d.conversation
		confirm = func(done func(error)) { p.updateConversation(target, map[string]any{"title": d.name}, done) }
	case dialogRenameProject:
		o.Title, o.Confirm = i18n.T("chat.renameProject"), i18n.T("chat.rename")
		o.ConfirmDisabled = strings.TrimSpace(d.name) == ""
		o.Body = func() { kit.TextInput(c, &d.name).FillWidth().Label(i18n.T("chat.newName")).AutoFocus() }
		id, name := d.project.ID, d.name
		confirm = func(done func(error)) { p.changeProject(id, &name, done) }
	case dialogDelete:
		o.Title, o.Confirm, o.Danger = i18n.T("chat.delete"), i18n.T("chat.delete"), true
		o.Message = i18n.T("chat.deleteDescription", "name", d.conversation.Title)
		id := d.conversation.ID
		confirm = func(done func(error)) { p.deleteConversation(id, done) }
	case dialogRemoveProject:
		o.Title, o.Confirm, o.Danger = i18n.T("chat.removeProject"), i18n.T("chat.remove"), true
		o.Message = i18n.T("chat.removeProjectHint", "name", d.project.Name)
		id := d.project.ID
		confirm = func(done func(error)) { p.changeProject(id, nil, done) }
	case dialogProject:
		o.Title = i18n.T("chat.openOrCreateProject")
		o.Confirm = i18n.T("chat.openProject")
		if d.create {
			o.Confirm = i18n.T("chat.create")
		}
		o.ConfirmDisabled = strings.TrimSpace(d.name) == "" || d.folder == ""
		o.Body = func() { p.projectFields(c, d) }
		create, name, folder, permission := d.create, d.name, d.folder, d.permission
		confirm = func(done func(error)) { p.saveProject(create, name, folder, permission, done) }
	case dialogLocations:
		o.Title, o.Confirm = i18n.T("chat.saveLocation"), i18n.T("chat.apply")
		o.Body = func() { p.locationFields(c, d) }
		working, saved := d.working, d.saved
		confirm = func(done func(error)) { p.saveLocations(working, saved, done) }
	}
	if kit.Dialog(c, &d.open, o) {
		p.run(func(done func(error)) {
			confirm(func(err error) {
				if err == nil {
					*d = dialogState{}
				}
				done(err)
			})
		})
	}
}

// projectFields are the project dialog's fields: open the folder chosen
// or create a folder in it, its name, and its default permission.
func (p *Chat) projectFields(c *ui.Context, d *dialogState) {
	t := c.Theme()
	ui.Checkbox(c, &d.create, i18n.T("chat.createProjectFolder")).Disabled(p.busy)
	kit.TextInput(c, &d.name).FillWidth().Label(i18n.T("chat.projectName")).Placeholder(i18n.T("chat.projectName")).AutoFocus().Disabled(p.busy)
	label := i18n.T("chat.location")
	if d.create {
		label = i18n.T("chat.projectParent")
	}
	useFolder := func(path string) {
		d.folder = path
		if !d.create {
			d.name = lastPathPart(path)
		}
	}
	zone := kit.Stack(c, label, "", func() {
		choose := kit.PathField(c, d.folder, i18n.T("chat.chooseLocation"), i18n.T("chat.chooseLocation"), p.busy)
		if choose.Clicked() {
			p.pickFolder(useFolder)
		}
	})
	p.dropFolder(c, zone, p.busy, useFolder)
	if d.folder != "" {
		destination := d.folder
		if d.create {
			separator := "/"
			if strings.Contains(d.folder, `\`) {
				separator = `\`
			}
			destination = strings.TrimRight(d.folder, `\/`) + separator + d.name
		}
		ui.Text(c, i18n.T("chat.projectDestination")+" · "+destination).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).Selectable()
	}
	kit.Stack(c, i18n.T("chat.projectPermission"), "", func() {
		value := string(d.permission)
		kit.Segmented(c, &value, []kit.SelectOption{
			{Value: string(model.PermissionAsk), Label: i18n.T("chat.projectPermissionAsk"), Icon: icons.ShieldQuestion},
			{Value: string(model.PermissionAuto), Label: i18n.T("chat.projectPermissionAuto"), Icon: icons.ShieldCheck},
			{Value: string(model.PermissionBypass), Label: i18n.T("chat.projectPermissionFull"), Icon: icons.ShieldOff},
		}, i18n.T("chat.projectPermission"))
		d.permission = model.Permission(value)
	})
}

func lastPathPart(path string) string {
	path = strings.TrimRight(path, `\/`)
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// locationFields are the working folder (a new chat outside projects
// only) and the save folder.
func (p *Chat) locationFields(c *ui.Context, d *dialogState) {
	newChat := p.conversation() == nil && p.newProjectID == ""
	working := kit.Stack(c, i18n.T("chat.sourcePath"), "", func() {
		choose := kit.PathField(c, d.working, i18n.T("chat.defaultLocation"), i18n.T("chat.chooseLocation"), p.busy || !newChat)
		if choose.Clicked() {
			p.pickFolder(func(path string) { d.working = path })
		}
	})
	p.dropFolder(c, working, p.busy || !newChat, func(path string) { d.working = path })
	saved := kit.Stack(c, i18n.T("chat.saveLocation"), i18n.T("chat.storageHint"), func() {
		choose := kit.PathField(c, d.saved, i18n.T("chat.defaultLocation"), i18n.T("chat.chooseLocation"), p.busy)
		if choose.Clicked() {
			p.pickFolder(func(path string) { d.saved = path })
		}
	})
	p.dropFolder(c, saved, p.busy, func(path string) { d.saved = path })
}

// moveDialog moves a conversation to another project.
func (p *Chat) moveDialog(c *ui.Context) {
	target := p.ui.move
	if p.ui.moveTo == nil {
		id := target.ProjectID
		p.ui.moveTo = &id
	}
	t, pal := c.Theme(), kit.Colors(c)
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: i18n.T("chat.moveToProject") + " · " + target.Title, Message: i18n.T("chat.moveDescription"),
		Cancel: i18n.T("chat.cancel"), Confirm: i18n.T("chat.moveConfirm"), Busy: p.busy, Error: p.err, Width: 480,
		ConfirmDisabled: *p.ui.moveTo == target.ProjectID,
		Body: func() {
			options := append([]Project{{ID: "", Name: i18n.T("chat.noProject"), Path: target.Directory}}, p.catalog.Projects...)
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, pr := range options {
					pr := pr
					b := ui.ButtonBase(c.Key("move-"+pr.ID)).FillWidth().Padding(8, 10).Gap(10).Radius(t.Radius).Justify(ui.Start).AlignItems(ui.Center).Label(pr.Name).Disabled(p.busy)
					chosen := *p.ui.moveTo == pr.ID
					if chosen {
						b.Background(pal.Selected)
					} else if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					b.Children(func() {
						ui.Icon(c, icons.FolderOpen).FontSize(16).Shrink(0)
						ui.Column(c).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, pr.Name)
							ui.Text(c, pr.Path).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).MinWidth(0)
						})
						if chosen {
							ui.Icon(c, icons.Check).FontSize(15).Shrink(0)
						}
					})
					if b.Clicked() {
						*p.ui.moveTo = pr.ID
					}
				}
			})
		}}) {
		id, projectID := target.ID, *p.ui.moveTo
		p.run(func(done func(error)) {
			p.moveConversation(id, projectID, func(err error) {
				if err == nil {
					p.ui.move, p.ui.moveTo = nil, nil
				}
				done(err)
			})
		})
	}
	if !open && !p.busy {
		p.ui.move, p.ui.moveTo = nil, nil
	}
}

// exportDialog saves a conversation as an archive: optionally with a
// password, and without media after the Host found it too large.
func (p *Chat) exportDialog(c *ui.Context) {
	target, e := p.ui.exportOf, &p.ui.exportState
	valid := !e.working && (!e.encrypt || len([]rune(e.password)) >= 4 && e.password == e.confirmation)
	confirm := i18n.T("chat.exportConfirm")
	if e.excludeMedia {
		confirm = i18n.T("chat.exportWithoutMedia")
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: i18n.T("chat.exportTask") + " · " + target.Title, Message: i18n.T("chat.exportDescription"),
		Cancel: i18n.T("chat.cancel"), Confirm: confirm, Busy: e.working, Error: e.err, ConfirmDisabled: !valid, Width: 480,
		Body: func() {
			if o := e.oversize; o != nil {
				kit.Notice(c, kit.Caution, "", i18n.T("chat.exportExcludeHint", "limit", math.Round(o.LimitBytes/1048576), "media", math.Round(o.MediaBytes/1048576)), nil)
			}
			ui.Checkbox(c, &e.encrypt, i18n.T("chat.exportEncrypt")).Disabled(e.working)
			if !e.encrypt {
				return
			}
			password := kit.TextInput(c, &e.password).FillWidth().Label(i18n.T("chat.exportPassword")).Placeholder(i18n.T("chat.exportPassword")).Disabled(e.working)
			again := kit.TextInput(c, &e.confirmation).FillWidth().Label(i18n.T("chat.exportPasswordAgain")).Placeholder(i18n.T("chat.exportPasswordAgain")).Disabled(e.working)
			if !e.show {
				password.Password()
				again.Password()
			}
			show := "chat.exportShowPassword"
			if e.show {
				show = "chat.exportHidePassword"
			}
			if kit.GhostButton(c, i18n.T(show), icons.Eye, false).Clicked() {
				e.show = !e.show
			}
			short := e.password != "" && len([]rune(e.password)) < 4
			if short || e.password != "" && e.confirmation != "" && e.password != e.confirmation {
				key := "chat.exportPasswordMismatch"
				if short {
					key = "chat.exportShortPassword"
				}
				kit.InlineError(c, i18n.T(key))
			}
		}}) && valid {
		e.working, e.err = true, ""
		p.transferBusy = true
		password := ""
		if e.encrypt {
			password = e.password
		}
		p.export(target.ID, password, e.excludeMedia, func(out exportOutcome, err error) {
			e.working, p.transferBusy = false, false
			switch {
			case err != nil:
				e.err = err.Error()
			case out.Status == "canceled":
			case out.Status == "oversize":
				if out.MediaBytes > 0 && !e.excludeMedia {
					e.oversize, e.excludeMedia = &out, true
				} else {
					e.err = i18n.T("chat.exportTooLarge", "limit", math.Round(out.LimitBytes/1048576))
				}
			default:
				p.notice = archiveNotice(out)
				p.ui.exportOf = nil
			}
		})
	}
	if !open && !e.working {
		p.ui.exportOf = nil
	}
}

// archiveNotice reports the archive's actual fidelity: excluded media is
// not a failed read, nor a complete archive.
func archiveNotice(o exportOutcome) string {
	key := "chat.exportDonePartial"
	switch {
	case o.Fidelity == "full":
		key = "chat.exportDoneFull"
	case o.Fidelity == "db-only":
		key = "chat.exportDoneDBOnly"
	case o.MediaExcluded > 0:
		key = "chat.exportDoneExcluded"
	}
	return i18n.T(key, "path", o.FilePath, "count", o.MediaExcluded, "missing", o.MediaMissing+float64(len(o.MissingFiles)))
}

// editState is the latest user message being edited and resent.
type editState struct {
	entry, text    string
	original       string
	width          float32
	stopping, busy bool
	reversible     bool
	restore        bool
	revision, err  string
	files          bool
}

// beginEdit edits the latest user message: the running turn stops first,
// and the turn's file changes say whether they can be restored.
func (p *Chat) beginEdit(m model.Message) {
	e := &editState{entry: m.ID, text: m.Text(), original: m.Text(), width: p.ui.userWidths[m.ID], stopping: p.view.Running, files: len(m.Attachments) > 0}
	p.ui.edit = e
	id := p.activeID
	review := func() {
		e.stopping = false
		chatCall(p, "review.turn", map[string]any{"sessionId": id, "turnId": m.ID}, func(r struct {
			Snapshot *struct {
				IsReversible   bool   `json:"isReversible"`
				WorkspaceState string `json:"workspaceState"`
				Revision       string `json:"revision"`
			} `json:"snapshot"`
		}, err error) {
			if err != nil {
				e.err = err.Error()
				return
			}
			if r.Snapshot != nil && r.Snapshot.IsReversible && r.Snapshot.WorkspaceState == "applied" {
				e.reversible, e.restore, e.revision = true, true, r.Snapshot.Revision
			}
		})
	}
	if !e.stopping {
		review()
		return
	}
	p.simple("stop", map[string]any{"id": id}, func(err error) {
		if err != nil {
			e.stopping, e.err = false, err.Error()
			return
		}
		review()
	})
}

// editInPlace preserves the bubble's intrinsic width, not the message/action
// row. Support controls live outside it and cannot stretch a short input.
func (p *Chat) editInPlace(c *ui.Context) {
	e := p.ui.edit
	width, _ := c.MeasureText(0, ui.Span{Text: e.text})
	width += 28
	if e.text == e.original && e.width > 0 {
		width = e.width
	}
	ui.Column(c.Key("edit:" + e.entry)).FillWidth().AlignItems(ui.End).Gap(8).Children(func() {
		ui.Column(c.Key("bubble")).Width(width).MaxWidthPercent(85).MinWidth(28).Padding(10, 14).Radius(14).Background(messageBackground(c)).Children(func() {
			input := ui.TextArea(c, &e.text).FillWidth().MinWidth(0).Lines(1, 12).MinHeight(24).MaxHeight(320).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent).LineHeight(1.5).
				Label(i18n.T("chat.editMessage")).AutoFocus().Disabled(e.busy)
			composing := input.Composing()
			input.OnTextKey(func(key ui.TextKey) bool {
				if composing {
					return key.Key == ui.KeyEnter || key.Key == ui.KeyEscape
				}
				if key.Key == ui.KeyEscape && !e.busy {
					p.ui.edit = nil
					return true
				}
				if key.Key == ui.KeyEnter && key.Mods&ui.Shift == 0 {
					e.text = key.Text()
					p.commitEdit(e)
					return true
				}
				return false
			})
		})
		ui.Column(c.Key("support")).MaxWidthPercent(85).MinWidth(0).Gap(8).AlignItems(ui.End).Children(func() {
			if e.reversible {
				ui.Checkbox(c, &e.restore, i18n.T("chat.editRestoreFiles")).Disabled(e.busy)
			} else {
				kit.Help(c, i18n.T("chat.editKeepFiles"))
			}
			if e.err != "" {
				kit.InlineError(c, e.err)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Wrap().Children(func() {
				if kit.GhostButton(c, i18n.T("chat.cancel"), nil, false).Disabled(e.busy).Clicked() {
					p.ui.edit = nil
				}
				if kit.PrimaryButton(c, i18n.T("chat.editResend"), nil, e.busy || e.stopping).Disabled(e.busy || e.stopping || isBlank(e.text) && !e.files).Clicked() {
					p.commitEdit(e)
				}
			})
		})
	})
}

// Both the native button and Enter use the same synchronous submission guard.
// A failed edit keeps the text and restore choice available for a retry.
func (p *Chat) commitEdit(e *editState) {
	if p.ui.edit != e || e.busy || e.stopping || isBlank(e.text) && !e.files {
		return
	}
	e.busy, e.err = true, ""
	revision := ""
	if e.restore {
		revision = e.revision
	}
	p.editMessage(e.entry, e.text, revision, func(err error) {
		e.busy = false
		if err != nil {
			e.err = err.Error()
			return
		}
		if p.ui.edit == e {
			p.ui.edit = nil
		}
	})
}

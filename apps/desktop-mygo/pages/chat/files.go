package chat

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// itemsState is what the transcript's native items keep between frames:
// the Host's file checks and turn reviews, the chosen change record, the
// answers' file references, the turn rows the scroll controls jump to,
// the plan reminder, the open tool dialog and the composer's pasted files.
type itemsState struct {
	// checks are file.stat results by conversation and path; wanted are
	// the paths this frame asked about that are not checked yet.
	checks map[string]*fileCheck
	wanted []string
	// reviews are review.turn results by conversation, turn and version.
	reviews map[string]*turnReview
	// changeTurn is the change record chosen; changesOpen and
	// generatedOpen show every row of the change card and every file chip.
	changeTurn                 string
	changesOpen, generatedOpen bool
	// errors are the failures of file actions, by the surface they came
	// from: "generated", a turn's "turn:" key or an answer's "answer:" key.
	errors map[string]string
	// refs are the file references of finished answers, by answer;
	// refsOpen shows all of an answer's.
	refs     map[string]answerRefs
	refsOpen map[string]bool
	markdown map[string]*markdown.Options
	// turnOrder are this frame's user turns, top to bottom; turnRows
	// their rows, which the scroll controls measure.
	turnOrder []string
	turnRows  map[string]*ui.Handle
	// content is the transcript's content column, which the rounds are
	// measured against.
	content ui.Handle
	// controls is the scroll controls' state from the last frame; jump
	// is where they scroll to next, after jumpWait's frame.
	controls scrollControls
	jump     *float32
	jumpWait bool
	// planCard is the latest turn's inline plan, planAnchor its entry;
	// the reminder hides once planDismissed names it.
	plan                      *model.Plan
	planCard                  ui.Handle
	planAnchor, planDismissed string
	planOpen                  bool
	tool                      *toolDialog
	// pasted are the image files the composer wrote for a chat not yet
	// created, deleted once no draft holds them.
	pasted map[string]bool
	// wasRunning notes a turn ending, after which files are checked again.
	wasRunning bool
}

// fileStat is a file the Host found, as file.stat reports it.
type fileStat struct {
	Path      string  `json:"path"`
	Name      string  `json:"name"`
	Directory bool    `json:"directory"`
	Size      float64 `json:"size"`
	Modified  float64 `json:"modified"`
}

// fileCheck is what file.stat said about a path: the file, missing, or
// why it cannot be read; transport marks the request itself failing.
type fileCheck struct {
	file      *fileStat
	missing   bool
	err       string
	transport bool
	pending   bool
}

func checkKey(id, path string) string { return id + "\x00" + filepath.ToSlash(path) }

// checkFile returns what the Host said about path, nil until it did; an
// unknown path is checked after the frame, with the others it asks
// about, as Cindy coalesces a render's existence checks.
func (p *Chat) checkFile(path string) *fileCheck {
	s := &p.ui.items
	if s.checks == nil {
		s.checks = map[string]*fileCheck{}
	}
	key := checkKey(p.activeID, path)
	check := s.checks[key]
	if check == nil {
		check = &fileCheck{pending: true}
		s.checks[key] = check
		s.wanted = append(s.wanted, path)
	}
	if check.pending {
		return nil
	}
	return check
}

// flushChecks sends the frame's new paths to file.stat, 64 a request.
func (p *Chat) flushChecks() {
	s := &p.ui.items
	id, wanted := p.activeID, s.wanted
	s.wanted = nil
	if id == "" {
		return
	}
	for start := 0; start < len(wanted); start += 64 {
		batch := wanted[start:min(start+64, len(wanted))]
		chatCall(p, "file.stat", map[string]any{"id": id, "paths": batch}, func(r struct {
			Files []struct {
				Requested string    `json:"requested"`
				File      *fileStat `json:"file"`
				Missing   bool      `json:"missing"`
				Error     string    `json:"error"`
			} `json:"files"`
		}, err error) {
			results := map[string]*fileCheck{}
			for _, f := range r.Files {
				results[f.Requested] = &fileCheck{file: f.File, missing: f.Missing, err: f.Error}
			}
			for _, path := range batch {
				check := results[path]
				switch {
				case err != nil:
					check = &fileCheck{err: err.Error(), transport: true}
				case check == nil:
					check = &fileCheck{err: i18n.T("chat.fileLinkUnavailable"), transport: true}
				}
				s.checks[checkKey(id, path)] = check
			}
		})
	}
}

// forgetChecks checks the conversation's files again when next shown, as
// after a turn changed them.
func (p *Chat) forgetChecks() {
	prefix := p.activeID + "\x00"
	for key, check := range p.ui.items.checks {
		if strings.HasPrefix(key, prefix) && !check.pending {
			delete(p.ui.items.checks, key)
		}
	}
}

// fileAction asks the Host to open or reveal a file of the conversation
// (file.open, file.reveal), reporting a failure on surface.
func (p *Chat) fileAction(method, path, surface string) {
	p.setItemError(surface, "")
	chatCall(p, method, map[string]any{"id": p.activeID, "path": path}, func(_ json.RawMessage, err error) {
		if err != nil {
			p.setItemError(surface, err.Error())
		}
	})
}

func (p *Chat) setItemError(surface, message string) {
	if p.ui.items.errors == nil {
		p.ui.items.errors = map[string]string{}
	}
	p.ui.items.errors[surface] = message
}

// copyFile puts the file itself on the clipboard, as Explorer's Copy.
func (p *Chat) copyFile(path, surface string) {
	data, err := transfer.FileData(path)
	if err != nil {
		p.setItemError(surface, err.Error())
		return
	}
	p.ui.services.WriteClipboardData(data)
}

// openPreview opens a file's preview address: in the side panel's
// browser, or in the system's.
func (p *Chat) openPreview(path, surface string, system bool) {
	p.setItemError(surface, "")
	chatCall(p, "file.preview", map[string]any{"id": p.activeID, "path": path}, func(r struct {
		URL string `json:"url"`
	}, err error) {
		switch {
		case err != nil:
			p.setItemError(surface, err.Error())
		case system && safeLink(r.URL):
			p.ui.services.OpenURL(r.URL)
		case system:
			p.setItemError(surface, i18n.T("chat.fileLinkUnavailable"))
		default:
			p.openURL(r.URL)
		}
	})
}

// fileChip is a file's chip: its icon and name, the path as its tip. A
// click opens it in the side panel; the context menu offers menu.
func fileChip(c *ui.Context, key, name, path string, directory bool, menu func(m *ui.Menu), activate func()) ui.Element {
	t, pal := c.Theme(), kit.Colors(c)
	chip := ui.ButtonBase(c.Key(key)).Height(28).MaxWidth(280).Gap(6).AlignItems(ui.Center).Padding(0, 10).Radius(99).
		Background(pal.Panel).Border(1, pal.Hairline).Label(name).Tooltip(path).ContextMenu(menu)
	if chip.Hovered() {
		chip.Background(t.SurfaceHover)
	}
	bounds := chip.Bounds()
	chip.HandleInput(func(event ui.InputEvent) bool { return fileMiddleClick(event, bounds, activate) })
	chip.Children(func() {
		icon := icons.FileText
		if directory {
			icon = icons.Folder
		}
		ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, name).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0)
	})
	return chip
}

// MyGo buttons already provide Enter/Space and the native keyboard context
// menu. Keep Cindy's middle-button activation on the same file authority; a
// release outside the chip is not activation.
func fileMiddleClick(event ui.InputEvent, bounds ui.Rect, activate func()) bool {
	if event.Button != 2 {
		return false
	}
	if event.Kind == ui.InputPointerDown {
		return true
	}
	if event.Kind != ui.InputPointerUp {
		return false
	}
	// Capture loss and window/menu takeover deliver a canceled release, not
	// a completed middle click. Let go without opening the protected file.
	if event.Canceled {
		return true
	}
	if event.X >= 0 && event.Y >= 0 && event.X < bounds.W && event.Y < bounds.H {
		activate()
	}
	return true
}

// moreToggle shows the rest of a list, or folds it again.
func moreToggle(c *ui.Context, open *bool, hidden int) {
	label := i18n.T("chat.generatedFilesMore", "count", hidden)
	icon := icons.ChevronDown
	if *open {
		label, icon = i18n.T("chat.generatedFilesLess"), icons.ChevronUp
	}
	if kit.GhostButton(c, label, icon, false).Expanded(*open).Clicked() {
		*open = !*open
	}
}

// generatedFiles are the chips of the files the conversation's tools
// wrote, as ChatGeneratedFiles: only files that still exist, six until
// asked for more, with a way to check again files that could not be read.
func (p *Chat) generatedFiles(c *ui.Context, paths []string) {
	s := &p.ui.items
	var files []fileStat
	failed, transport := 0, ""
	for _, path := range paths {
		check := p.checkFile(path)
		switch {
		case check == nil:
		case check.file != nil && !check.file.Directory:
			files = append(files, *check.file)
		case check.err != "":
			failed++
			if check.transport {
				transport = check.err
			}
		}
	}
	message := first(s.errors["generated"], transport)
	if message == "" && failed > 0 {
		message = i18n.T("chat.generatedFilesUnavailable", "count", failed)
	}
	if len(files) == 0 && message == "" {
		return
	}
	ui.Column(c.Key("generated")).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			shown := files
			if !s.generatedOpen && len(files) > 6 {
				shown = files[:6]
			}
			for _, f := range shown {
				path := f.Path
				chip := fileChip(c, "generated:"+path, f.Name, path, false, func(m *ui.Menu) {
					if m.Item(i18n.T("chat.reviewOpenFile")).Chosen() {
						p.openFile(path, 0)
					}
					if m.Item(i18n.T("chat.copyPath")).Chosen() {
						p.copy(path)
					}
					if m.Item(i18n.T("chat.revealFile")).Chosen() {
						p.fileAction("file.reveal", path, "generated")
					}
					if m.Item(i18n.T("chat.openSystem")).Chosen() {
						p.fileAction("file.open", path, "generated")
					}
				}, func() { p.openFile(path, 0) })
				if chip.Clicked() {
					p.openFile(path, 0)
				}
			}
			if len(files) > 6 {
				moreToggle(c, &s.generatedOpen, len(files)-6)
			}
		})
		if message != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				kit.Status(c, message).MinWidth(0)
				if kit.IconButton(c, icons.RefreshCw, i18n.T("chat.retry")).Clicked() {
					p.setItemError("generated", "")
					p.forgetChecks()
				}
			})
		}
	})
}

// turnReview is a turn's review.turn: its snapshot and each file's
// patch, or why it could not be read; applying marks an undo or reapply
// on its way.
type turnReview struct {
	Snapshot struct {
		IsReversible   bool   `json:"isReversible"`
		WorkspaceState string `json:"workspaceState"`
		Root           string `json:"root"`
		Revision       string `json:"revision"`
		Files          []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"files"`
	} `json:"snapshot"`
	Patches  map[string]string `json:"patches"`
	loaded   bool
	err      string
	applying bool
}

// review returns a turn's changes, read once per version of the turn.
func (p *Chat) review(turn model.ChangedTurn) *turnReview {
	s := &p.ui.items
	if s.reviews == nil {
		s.reviews = map[string]*turnReview{}
	}
	key := p.activeID + "\x00" + turn.TurnID + "\x00" + turn.Version
	if r := s.reviews[key]; r != nil {
		return r
	}
	r := &turnReview{}
	s.reviews[key] = r
	chatCall(p, "review.turn", map[string]any{"sessionId": p.activeID, "turnId": turn.TurnID}, func(value turnReview, err error) {
		if err != nil {
			r.err = err.Error()
			return
		}
		r.Snapshot, r.Patches, r.loaded = value.Snapshot, value.Patches, true
	})
	return r
}

// absolute is a reviewed file's path in the turn's folder.
func (r *turnReview) absolute(path string) string {
	if windowsPath(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return path
	}
	return strings.TrimRight(r.Snapshot.Root, `/\`) + "/" + path
}

func windowsPath(path string) bool {
	return len(path) >= 2 && path[1] == ':' && (path[0]|0x20 >= 'a' && path[0]|0x20 <= 'z')
}

// applyTurn undoes the turn's changes, or applies them again, then reads
// the conversation's files afresh.
func (p *Chat) applyTurn(turnID string, r *turnReview) {
	if r.applying || !r.loaded {
		return
	}
	action := "undo"
	if r.Snapshot.WorkspaceState == "undone" {
		action = "reapply"
	}
	r.applying, r.err = true, ""
	chatCall(p, "review.turn.apply", map[string]any{"sessionId": p.activeID, "turnId": turnID,
		"mutation": map[string]any{"revision": r.Snapshot.Revision, "action": action}}, func(snapshot json.RawMessage, err error) {
		r.applying = false
		if err == nil {
			err = json.Unmarshal(snapshot, &r.Snapshot)
		}
		if err != nil {
			r.err = err.Error()
			return
		}
		p.forgetChecks()
	})
}

// turnChanges is the transcript's one change card, as
// ChatTranscriptChanges: every recorded turn stays reachable through the
// record choice, and each keeps its own undo.
func (p *Chat) turnChanges(c *ui.Context, turns []model.ChangedTurn) {
	s := &p.ui.items
	turn := turns[len(turns)-1]
	index := len(turns) - 1
	for i, value := range turns {
		if value.TurnID == s.changeTurn {
			turn, index = value, i
		}
	}
	t := c.Theme()
	record := func(i int, value model.ChangedTurn) string {
		label := i18n.T("chat.changeRecord", "number", i+1)
		if title := strings.Join(strings.Fields(value.Title), " "); title != "" {
			label += " · " + title
		}
		return label
	}
	ui.Column(c.Key("changes")).FillWidth().Gap(8).Children(func() {
		if len(turns) > 1 {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, i18n.T("chat.changeRecords")).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).Shrink(0)
				current := record(index, turn)
				choose := ui.ButtonBase(c).Grow(1).Basis(220).MinWidth(0).Gap(8).AlignItems(ui.Center).Padding(5, 8).Radius(t.Radius).
					Border(1, t.Border).Label(i18n.T("chat.changeRecords")).Tooltip(turn.Title)
				choose.Children(func() {
					ui.Text(c, current).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
					ui.Icon(c, icons.ChevronDown).FontSize(14).Shrink(0)
				})
				choose.Menu(func(m *ui.Menu) {
					for i, value := range turns {
						if m.Item(record(i, value)).Radio(value.TurnID == turn.TurnID).Chosen() {
							s.changeTurn = value.TurnID
						}
					}
				})
			})
		}
		p.turnCard(c, turn)
	})
}

// turnCard is one turn's changes, as ChatTurnChanges: the count of files
// and lines, undo or reapply, the review, and the first three files.
func (p *Chat) turnCard(c *ui.Context, turn model.ChangedTurn) {
	r := p.review(turn)
	if !r.loaded && r.err == "" || r.loaded && len(r.Snapshot.Files) == 0 && r.err == "" {
		return
	}
	t, pal := c.Theme(), kit.Colors(c)
	type row struct {
		path     string
		add, del int
	}
	var rows []row
	additions, deletions := 0, 0
	for _, f := range r.Snapshot.Files {
		a, d := model.PatchCounts(r.Patches[f.Path])
		rows = append(rows, row{path: f.Path, add: a, del: d})
		additions, deletions = additions+a, deletions+d
	}
	surface := "turn:" + turn.TurnID
	counts := func(a, d int) {
		kit.Mono(c, "+"+kit.Itoa(a)+" −"+kit.Itoa(d)).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Shrink(0)
	}
	ui.Column(c.Key("turn-card:"+turn.TurnID)).FillWidth().Radius(8).Border(1, pal.Hairline).Background(pal.Panel).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Padding(6, 10).MinHeight(40).Children(func() {
			ui.Icon(c, icons.FileDiff).FontSize(18).TextColor(t.TextMuted).Shrink(0)
			ui.Row(c).Grow(1).Basis(100).MinWidth(0).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
				title := i18n.T("chat.turnReview")
				if r.loaded {
					title = i18n.T("chat.turnFilesChanged", "count", len(rows))
				}
				ui.Text(c, title).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
				if len(rows) > 0 {
					counts(additions, deletions)
				}
			})
			if r.loaded && r.Snapshot.IsReversible {
				label, icon := i18n.T("chat.turnUndo"), icons.Undo2
				if r.Snapshot.WorkspaceState == "undone" {
					label, icon = i18n.T("chat.turnReapply"), icons.Redo2
				}
				disabled := p.view.Running || p.busy || r.applying || r.Snapshot.WorkspaceState == "conflict"
				if kit.Button(c, label, icon, r.applying).Disabled(disabled).Clicked() {
					p.applyTurn(turn.TurnID, r)
				}
			}
			if kit.Button(c, i18n.T("chat.reviewViewChanges"), nil, false).Clicked() {
				p.openReview(turn.TurnID, "")
			}
		})
		if len(rows) > 0 {
			ui.Column(c).FillWidth().Padding(4).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
				shown := rows
				if !p.ui.items.changesOpen && len(rows) > 3 {
					shown = rows[:3]
				}
				for _, f := range shown {
					path, abs := f.path, r.absolute(f.path)
					line := ui.ButtonBase(c.Key("turn-file:"+path)).FillWidth().Height(28).Gap(8).AlignItems(ui.Center).Padding(0, 8).Radius(6).
						Label(path).Tooltip(path).ContextMenu(func(m *ui.Menu) {
						if m.Item(i18n.T("chat.reviewViewChanges")).Chosen() {
							p.openReview(turn.TurnID, path)
						}
						if m.Item(i18n.T("chat.reviewOpenFile")).Chosen() {
							p.openFile(abs, 0)
						}
						if m.Item(i18n.T("chat.copyPath")).Chosen() {
							p.copy(abs)
						}
						if m.Item(i18n.T("chat.revealFile")).Chosen() {
							p.fileAction("file.reveal", abs, surface)
						}
						if m.Item(i18n.T("chat.openSystem")).Chosen() {
							p.fileAction("file.open", abs, surface)
						}
					})
					bounds := line.Bounds()
					line.HandleInput(func(event ui.InputEvent) bool {
						return fileMiddleClick(event, bounds, func() { p.openReview(turn.TurnID, path) })
					})
					if line.Hovered() {
						line.Background(t.SurfaceHover)
					}
					line.Children(func() {
						ui.Icon(c, icons.FileText).FontSize(15).TextColor(t.TextMuted).Shrink(0)
						ui.Text(c, path).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
						counts(f.add, f.del)
					})
					if line.Clicked() {
						p.openReview(turn.TurnID, path)
					}
				}
				if len(rows) > 3 {
					moreToggle(c, &p.ui.items.changesOpen, len(rows)-3)
				}
			})
		}
		if r.Snapshot.WorkspaceState == "conflict" {
			kit.Notice(c, kit.Caution, "", i18n.T("chat.turnConflict"), nil)
		}
		if r.loaded && !r.Snapshot.IsReversible {
			kit.Status(c, i18n.T("chat.turnCapturePartial"))
		}
		if message := first(r.err, p.ui.items.errors[surface]); message != "" {
			kit.InlineError(c, message)
		}
	})
}

// conversationFiles is the transcript's end: the files the turns wrote
// and their changes, an area of its own that no reply owns.
func (p *Chat) conversationFiles(c *ui.Context, items []*model.Item) {
	generated := model.GeneratedPaths(items)
	changed := model.ChangedTurns(items, p.visibleMessages())
	if len(generated) == 0 && len(changed) == 0 {
		return
	}
	ui.Column(c.Key("conversation-files")).FillWidth().Gap(16).Margin(16, 0, 0, 0).Label(i18n.T("chat.conversationFiles")).Children(func() {
		if len(generated) > 0 {
			p.generatedFiles(c, generated)
		}
		if len(changed) > 0 {
			p.turnChanges(c, changed)
		}
	})
}

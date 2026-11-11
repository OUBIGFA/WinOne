package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type ReviewFile struct {
	Path                 string `json:"path"`
	OldPath              string `json:"oldPath"`
	Status               string `json:"status"`
	Binary               bool   `json:"binary"`
	Untracked            bool   `json:"untracked"`
	Additions, Deletions int
}
type ReviewSnapshot struct {
	Root, Branch, Scope, Base, Revision, Source string
	Files                                       []ReviewFile
	IsReversible                                bool
	WorkspaceState                              string
}
type ReviewMetadata struct {
	Repository                      bool
	Root, Branch, HeadOID, Upstream string
	Detached, Unborn                bool
	Ahead, Behind                   int
	Candidates                      []struct {
		RefName, ShortName, OID string
		IsDefaultBranch         bool
	}
	InProgress []struct{ Kind, Marker, Path string }
}
type ReviewCommit struct{ ID, Title, Author, Date string }
type ReviewLease struct {
	RemoteRef   string `json:"remoteRef"`
	ExpectedOID string `json:"expectedOid"`
}
type ReviewPushResult struct {
	Kind, Remote, RemoteRef, RemoteOID, Stdout, Stderr string
	Ahead, Behind                                      int
}
type ReviewOptions struct {
	Prefix, SessionID, TurnID, InitialPath string
	Running                                func() bool
	OnOpenFile                             func(string, int)
	OnComment                              func(string) error
}

// ReviewPane uses the original Host snapshots and mutation/force leases. The
// MyGo diff is presentation only: its in-memory Keep/Revert buttons are off.
type ReviewPane struct {
	env                                *core.Env
	options                            ReviewOptions
	ctx                                context.Context
	cancel                             context.CancelFunc
	snapshot                           ReviewSnapshot
	metadata                           ReviewMetadata
	commits                            []ReviewCommit
	scope, base, selected, filter      string
	loading, diffLoading, busy, closed bool
	refreshAgain                       bool
	diffCancel                         context.CancelFunc
	generation, diffGeneration         uint64
	err                                string
	diff                               *editor.DiffState
	patch                              *editor.FilePatch
	rawPatch                           string
	hunk                               string
	layout                             string
	ignoreWhitespace                   bool
	split                              kit.SplitState
	confirm                            *reviewConfirmation
	comment, commitMessage             string
	includeUnstaged                    bool
	commitOpen                         bool
}
type reviewConfirmation struct {
	action   string
	paths    []string
	revision string
	hunk     *int
	lease    *ReviewLease
	message  string
}

func NewReviewPane(env *core.Env, options ReviewOptions) *ReviewPane {
	ctx, cancel := context.WithCancel(context.Background())
	p := &ReviewPane{env: env, options: options, ctx: ctx, cancel: cancel, scope: "working", hunk: "all", layout: "unified", selected: options.InitialPath, split: kit.SplitState{Percent: 30}}
	p.Refresh()
	return p
}
func (p *ReviewPane) Close() {
	p.closed = true
	p.cancel()
	if p.diffCancel != nil {
		p.diffCancel()
	}
	p.generation++
	p.diffGeneration++
}
func (p *ReviewPane) Busy() bool    { return p.busy }
func (p *ReviewPane) running() bool { return p.options.Running != nil && p.options.Running() }
func (p *ReviewPane) call(method string, params map[string]any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	params["sessionId"] = p.options.SessionID
	return p.env.Host.CallInto(p.options.Prefix+"review."+method, params, out)
}
func (p *ReviewPane) Refresh() {
	if p.closed {
		return
	}
	if p.loading {
		p.refreshAgain = true
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	scope, base, turn := p.scope, p.base, p.options.TurnID
	type result struct {
		snapshot ReviewSnapshot
		metadata ReviewMetadata
		commits  []ReviewCommit
	}
	core.Go(p.env, func() (result, error) {
		var r result
		if err := p.call("snapshot", map[string]any{"scope": scope, "base": base, "turnId": turn}, &r.snapshot); err != nil {
			return r, err
		}
		if err := p.call("metadata", nil, &r.metadata); err != nil {
			return r, err
		}
		if r.metadata.Repository && turn == "" {
			if err := p.call("commits", nil, &r.commits); err != nil {
				return r, err
			}
		}
		return r, nil
	}, func(r result, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		defer func() {
			if p.refreshAgain {
				p.refreshAgain = false
				p.Refresh()
			}
		}()
		if err != nil {
			p.err = err.Error()
			return
		}
		p.snapshot, p.metadata, p.commits = r.snapshot, r.metadata, r.commits
		p.err = ""
		found := false
		for _, file := range r.snapshot.Files {
			if file.Path == p.selected {
				found = true
			}
		}
		if !found {
			p.selected = ""
			if len(r.snapshot.Files) > 0 {
				p.selected = r.snapshot.Files[0].Path
			}
		}
		p.loadDiff()
	})
}
func (p *ReviewPane) chooseScope(scope, base string) {
	if p.scope == scope && p.base == base {
		return
	}
	p.scope, p.base = scope, base
	p.generation++
	p.loading = false
	p.selected = ""
	p.Refresh()
}
func (p *ReviewPane) file() *ReviewFile {
	for i := range p.snapshot.Files {
		if p.snapshot.Files[i].Path == p.selected {
			return &p.snapshot.Files[i]
		}
	}
	return nil
}
func (p *ReviewPane) loadDiff() {
	if p.diffCancel != nil {
		p.diffCancel()
		p.diffCancel = nil
	}
	p.diffGeneration++
	generation := p.diffGeneration
	p.hunk = "all"
	p.patch = nil
	p.diff = nil
	p.rawPatch = ""
	file := p.file()
	if p.closed || file == nil || file.Binary {
		p.diffLoading = false
		return
	}
	scope, base, revision, path, turn, ignore := p.snapshot.Scope, p.snapshot.Base, p.snapshot.Revision, file.Path, p.options.TurnID, p.ignoreWhitespace
	p.diffLoading = true
	ctx, cancel := context.WithCancel(p.ctx)
	p.diffCancel = cancel
	core.Go(p.env, func() (string, error) {
		var location struct {
			URL string `json:"url"`
		}
		err := p.call("diff.preview", map[string]any{"scope": scope, "base": base, "path": path, "revision": revision, "turnId": turn, "ignoreWhitespace": ignore}, &location)
		if err != nil {
			return "", err
		}
		data, err := FetchPreview(ctx, location.URL, 32<<20)
		return string(data), err
	}, func(raw string, err error) {
		if p.closed || generation != p.diffGeneration {
			return
		}
		p.diffLoading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.rawPatch = raw
		files, err := editor.ParsePatch(raw)
		if err != nil {
			p.err = err.Error()
			return
		}
		if len(files) == 0 {
			p.err = ""
			return
		}
		if len(files) != 1 {
			p.err = wt("invalidDiff")
			return
		}
		p.patch = &files[0]
		p.diff = &editor.DiffState{Wrap: true, Layout: editor.DiffUnified}
		p.diff.SetPatch(files[0])
		if p.layout == "split" {
			p.diff.Layout = editor.DiffSplit
		}
		p.err = ""
	})
}
func (p *ReviewPane) requestMutation(action string, all bool) {
	if p.busy || p.loading || p.diffLoading || p.running() {
		p.err = wt("stopBeforeReview")
		return
	}
	targets := []ReviewFile{}
	if all {
		targets = append(targets, p.snapshot.Files...)
	} else if file := p.file(); file != nil {
		targets = append(targets, *file)
	}
	if len(targets) == 0 {
		return
	}
	paths := []string{}
	for _, file := range targets {
		paths = append(paths, file.Path)
		if file.OldPath != "" && file.OldPath != file.Path {
			paths = append(paths, file.OldPath)
		}
	}
	c := &reviewConfirmation{action: action, paths: paths, revision: p.snapshot.Revision}
	if !all && p.hunk != "all" {
		if p.ignoreWhitespace {
			p.err = wt("canonicalHunks")
			return
		}
		index, err := strconv.Atoi(p.hunk)
		if err != nil || p.patch == nil || index < 0 || index >= len(p.patch.Hunks) {
			p.err = wt("invalidDiff")
			return
		}
		c.hunk = &index
	}
	if action == "revert" {
		c.message = wt("revertWarning")
		p.confirm = c
		return
	}
	p.mutate(c)
}
func (p *ReviewPane) mutate(action *reviewConfirmation) {
	if p.busy || p.closed || p.running() {
		p.err = wt("stopBeforeReview")
		return
	}
	p.busy = true
	p.err = ""
	params := map[string]any{"action": action.action, "paths": action.paths, "revision": action.revision}
	if action.hunk != nil {
		params["hunk"] = *action.hunk
	}
	if action.action == "commit" {
		params["message"] = p.commitMessage
		params["includeUnstaged"] = p.includeUnstaged
	}
	method := "mutate"
	body := map[string]any{"mutation": params}
	if action.action == "undo" || action.action == "redo" {
		method = "turn.apply"
		body["turnId"] = p.options.TurnID
	}
	core.Do(p.env, func() error { return p.call(method, body, nil) }, func(err error) {
		p.busy = false
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.confirm = nil
		p.commitOpen = false
		p.Refresh()
	})
}
func (p *ReviewPane) push(lease *ReviewLease) {
	if p.busy || p.closed || p.running() {
		p.err = wt("stopBeforeReview")
		return
	}
	p.busy = true
	p.err = ""
	body := map[string]any{}
	if lease != nil {
		copy := *lease
		body["confirmForce"] = copy
	}
	core.Go(p.env, func() (ReviewPushResult, error) {
		var result ReviewPushResult
		err := p.call("push", body, &result)
		return result, err
	}, func(result ReviewPushResult, err error) {
		p.busy = false
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		if result.Kind == "needs-force" {
			p.confirm = &reviewConfirmation{action: "push", lease: &ReviewLease{RemoteRef: result.RemoteRef, ExpectedOID: result.RemoteOID}, message: wt("forceWarning", "remote", result.Remote, "ref", result.RemoteRef, "ahead", result.Ahead, "behind", result.Behind)}
			return
		}
		if result.Kind != "pushed" {
			p.err = wt("invalidPushReceipt")
			return
		}
		p.confirm = nil
		p.Refresh()
	})
}
func (p *ReviewPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		p.toolbar(c)
		if p.err != "" {
			ui.Column(c).FillWidth().Padding(0, 12, 8, 12).Children(func() { kit.InlineError(c, p.err) })
		}
		if p.loading {
			ui.Row(c).Padding(0, 12, 8, 12).Children(func() { kit.Loader(c) })
		}
		kit.Split(c, &p.split, kit.SplitOptions{Label: wt("resizeReview"), MinFirst: 20, MinSecond: 40}, func() { p.filesView(c) }, func() { p.diffView(c) }).Grow(1).MinHeight(0)
	})
	p.dialogs(c)
}
func (p *ReviewPane) toolbar(c *ui.Context) {
	ui.Column(c).FillWidth().Shrink(0).Gap(6).Padding(4, 8, 4, 12).Children(func() {
		if p.options.TurnID == "" && p.snapshot.Source != "session" {
			scope := p.scope
			if changed, _ := kit.Select(c, &scope, []kit.SelectOption{{Value: "working", Label: wt("working")}, {Value: "staged", Label: wt("staged")}, {Value: "branch", Label: wt("branch")}, {Value: "commit", Label: wt("history")}}); changed {
				base := ""
				if scope == "branch" && len(p.metadata.Candidates) > 0 {
					base = p.metadata.Candidates[0].RefName
				}
				if scope == "commit" && len(p.commits) > 0 {
					base = p.commits[0].ID
				}
				p.chooseScope(scope, base)
			}
			if p.scope == "branch" || p.scope == "commit" {
				options := []kit.SelectOption{}
				if p.scope == "branch" {
					for _, branch := range p.metadata.Candidates {
						options = append(options, kit.SelectOption{Value: branch.RefName, Label: branch.ShortName})
					}
				} else {
					for _, commit := range p.commits {
						options = append(options, kit.SelectOption{Value: commit.ID, Label: commit.Title})
					}
				}
				base := p.base
				if changed, _ := kit.Select(c, &base, options); changed {
					p.chooseScope(p.scope, base)
				}
			}
		}
		ui.Row(c).FillWidth().MinHeight(36).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
				if p.snapshot.Branch != "" {
					ui.Icon(c, icons.GitBranch).FontSize(14).TextColor(c.Theme().TextMuted).Shrink(0)
				}
				ui.Text(c, p.snapshot.Branch).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(p.snapshot.Root)
			})
			if toolButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Disabled(p.loading || p.busy).Clicked() {
				p.Refresh()
			}
			if p.metadata.Repository && p.options.TurnID == "" {
				if kit.Button(c, wt("commit"), icons.GitCommitHorizontal, false).Disabled(p.running() || p.busy).Clicked() {
					p.commitOpen = true
				}
				if kit.Button(c, wt("push"), icons.Upload, false).Disabled(p.running() || p.busy).Clicked() {
					p.push(nil)
				}
			}
			if p.snapshot.IsReversible && p.options.TurnID != "" {
				action := "undo"
				if p.snapshot.WorkspaceState == "undone" {
					action = "redo"
				}
				if kit.Button(c, wt(action), icons.Undo2, false).Disabled(p.running() || p.busy).Clicked() {
					p.confirm = &reviewConfirmation{action: action, revision: p.snapshot.Revision, message: wt("turnWarning")}
				}
			}
		})
		for _, operation := range p.metadata.InProgress {
			kit.Help(c, wt("gitOperation", "kind", operation.Kind))
		}
		if p.running() {
			kit.Help(c, wt("stopBeforeReview"))
		}
	})
}
func (p *ReviewPane) filesView(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Padding(0, 8, 8, 8).Gap(6).Children(func() {
		kit.TextInput(c, &p.filter).Label(wt("filter")).Placeholder(wt("filter")).FillWidth()
		if p.metadata.Repository && (p.scope == "working" || p.scope == "staged") && p.options.TurnID == "" {
			action := "stage"
			if p.scope == "staged" {
				action = "unstage"
			}
			if kit.Button(c, wt(action+"All"), icons.Check, false).Disabled(p.running() || p.busy || len(p.snapshot.Files) == 0).Clicked() {
				p.hunk = "all"
				p.requestMutation(action, true)
			}
		}
		files := []ReviewFile{}
		for _, file := range p.snapshot.Files {
			if p.filter == "" || strings.Contains(strings.ToLower(file.Path), strings.ToLower(p.filter)) {
				files = append(files, file)
			}
		}
		ui.List(c, nil, len(files), func(i int) {
			file := files[i]
			b := ui.ButtonBase(c).FillWidth().MinWidth(0).Padding(6, 10).Radius(c.Theme().Radius).Label(file.Path)
			if p.selected == file.Path {
				b.Background(kit.Colors(c).Selected)
			} else if b.Hovered() {
				b.Background(c.Theme().SurfaceHover)
			}
			b.Children(func() {
				ui.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
					// The file's name leads; its folder and counts follow.
					ui.Text(c, filepath.Base(file.Path)).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(file.Path)
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						if dir := filepath.ToSlash(filepath.Dir(file.Path)); dir != "." {
							kit.Caption(c, dir).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
						}
						kit.Mono(c, fmt.Sprintf("+%d −%d", file.Additions, file.Deletions)).FontSize(kit.CaptionSize(c)).TextColor(c.Theme().TextMuted).SingleLine().Shrink(0)
					})
				})
			})
			if b.Clicked() {
				p.selected = file.Path
				p.loadDiff()
			}
		}).Grow(1).MinHeight(0).FillWidth()
	})
}
func (p *ReviewPane) diffView(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Padding(0, 8, 8, 8).Gap(6).Children(func() {
		file := p.file()
		if file == nil {
			emptyHint(c, icons.FileDiff, wt("chooseReviewFile"))
			return
		}
		// One line of view options: the file's actions as icons, how the
		// difference is laid out, and whether blank changes count.
		ui.Row(c).FillWidth().MinHeight(36).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				if toolButton(c, icons.FileText, wt("openFile")).Clicked() && p.options.OnOpenFile != nil {
					path := file.Path
					if !filepath.IsAbs(path) {
						path = filepath.Join(p.snapshot.Root, path)
					}
					p.options.OnOpenFile(path, 0)
				}
				if toolButton(c, icons.Copy, wt("copyPatch")).Disabled(p.rawPatch == "").Clicked() {
					c.Services().WriteClipboard(p.rawPatch)
				}
			})
			if kit.Segmented(c, &p.layout, []kit.SelectOption{{Value: "unified", Label: wt("unified")}, {Value: "split", Label: wt("split")}}, wt("unified")+" / "+wt("split")) && p.diff != nil {
				if p.layout == "split" {
					p.diff.Layout = editor.DiffSplit
				} else {
					p.diff.Layout = editor.DiffUnified
				}
			}
			if ui.Checkbox(c, &p.ignoreWhitespace, wt("ignoreWhitespace")).Changed() {
				p.loadDiff()
			}
		})
		if p.metadata.Repository && (p.scope == "working" || p.scope == "staged") && p.options.TurnID == "" {
			options := []kit.SelectOption{{Value: "all", Label: wt("wholeFile")}}
			if p.patch != nil && !p.ignoreWhitespace {
				for i := range p.patch.Hunks {
					options = append(options, kit.SelectOption{Value: strconv.Itoa(i), Label: wt("hunk", "count", i+1)})
				}
			}
			if p.ignoreWhitespace {
				kit.Help(c, wt("canonicalHunks"))
			}
			kit.Select(c, &p.hunk, options)
			ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
				action := "stage"
				if p.scope == "staged" {
					action = "unstage"
				}
				if kit.Button(c, wt(action), icons.Check, false).Disabled(p.running() || p.busy).Clicked() {
					p.requestMutation(action, false)
				}
				if p.scope == "working" && kit.Button(c, wt("revert"), icons.Undo2, false).Disabled(p.running() || p.busy).Clicked() {
					p.requestMutation("revert", false)
				}
			})
		}
		if p.diffLoading {
			kit.Loader(c)
		}
		if p.diff != nil {
			editor.DiffView(c, p.diff).Grow(1).MinHeight(0).FillWidth()
		} else if file.Binary {
			kit.Help(c, wt("binaryDiff"))
		}
		if p.options.OnComment != nil {
			ui.TextArea(c, &p.comment).FillWidth().Height(70).Placeholder(wt("comment")).Label(wt("comment"))
			if kit.Button(c, wt("sendComment"), icons.MessageSquarePlus, false).Disabled(strings.TrimSpace(p.comment) == "").Clicked() {
				if err := p.options.OnComment("Review " + file.Path + ":\n" + p.comment); err != nil {
					p.err = err.Error()
				} else {
					p.comment = ""
				}
			}
		}
	})
}
func (p *ReviewPane) dialogs(c *ui.Context) {
	if p.commitOpen {
		if kit.Dialog(c, &p.commitOpen, kit.DialogOptions{Title: wt("commit"), Confirm: wt("commit"), Cancel: i18n.T("chat.cancel"), Busy: p.busy, Error: p.err, ConfirmDisabled: strings.TrimSpace(p.commitMessage) == "" || p.running(), Body: func() {
			ui.TextArea(c, &p.commitMessage).FillWidth().Height(100).Label(wt("commitMessage")).Disabled(p.busy)
			kit.Toggle(c, &p.includeUnstaged, wt("includeUnstaged"), "").Disabled(p.busy)
		}}) {
			p.mutate(&reviewConfirmation{action: "commit"})
		}
	}
	if action := p.confirm; action != nil {
		open := true
		title := wt(action.action)
		confirm := wt("confirm")
		if action.lease != nil {
			title = wt("forcePush")
			confirm = title
		}
		if kit.Dialog(c, &open, kit.DialogOptions{Title: title, Message: action.message, Confirm: confirm, Cancel: i18n.T("chat.cancel"), Danger: true, Busy: p.busy, Error: p.err, ConfirmDisabled: p.running()}) {
			if action.action == "push" {
				p.push(action.lease)
			} else {
				p.mutate(action)
			}
		}
		if !open {
			p.confirm = nil
		}
	}
}

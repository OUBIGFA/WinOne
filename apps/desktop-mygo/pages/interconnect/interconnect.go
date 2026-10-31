// Package interconnect is the Interconnect page: sending and receiving
// files and text, cloud sharing, port mapping, and the sharing settings.
// The transfers run in the Host's supervised workers; the window names
// paths and shows their state.
//
// React source: features/interconnect.tsx, interconnect-general.tsx,
// interconnect-transfer.tsx and interconnect-peer.tsx. A share's page
// shows in a web view below its task, as React's iframe did.
package interconnect

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
)

func init() {
	shell.Register("interconnect", core.Settings, func(env *core.Env) core.Page { return newPage(env) })
}

type preferences struct {
	Bind        string  `json:"bind"`
	Port        float64 `json:"port"`
	DownloadDir string  `json:"downloadDir"`
}

type address struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip"`
	URL  string `json:"url,omitempty"`
	QR   string `json:"qr,omitempty"`
}

type state struct {
	Running   bool        `json:"running"`
	Settings  preferences `json:"settings"`
	Addresses []address   `json:"addresses"`
}

type general struct {
	Notify      bool   `json:"notify"`
	Clipboard   bool   `json:"clipboard"`
	DownloadDir string `json:"downloadDir"`
}

const (
	tabFiles = iota
	tabStore
	tabTunnel
	tabGeneral
)

// clipboardCode is a transfer code found on the clipboard.
type clipboardCode struct{ kind, code string }

var transferCode = regexp.MustCompile(`(?i)^[0-9]+-[a-z0-9-]+$`)

// classify tells a share address, a cloud share or a croc code apart.
func classify(code string) string {
	switch {
	case strings.HasPrefix(code, "tc") && len(code) > 40:
		return "share"
	case strings.HasPrefix(code, "croc-store-v1.") || strings.HasPrefix(code, "https://getcroc.com/s/") || strings.HasPrefix(code, "https://gofile.io/d/"):
		return "store"
	case transferCode.MatchString(code):
		return "transfer"
	}
	return ""
}

type page struct {
	env        *core.Env
	tab        int
	state      *state
	draft      *preferences
	general    *general
	draftGen   *general
	busy       bool
	err        string
	focuses    int
	clipboard  *clipboardCode
	files      *transferPanel
	store      *transferPanel
	tunnel     *peerPanel
	settings   *generalPanel
	readFocus  bool
}

func newPage(env *core.Env) *page {
	p := &page{env: env, files: newTransferPanel(env, false), store: newTransferPanel(env, true), tunnel: newPeerPanel(env, "tunnel"),
		settings: &generalPanel{env: env}, focuses: -1}
	p.reload(true)
	return p
}

// reload reads the sharing state and settings; reset replaces the draft.
func (p *page) reload(reset bool) {
	core.Go(p.env, func() (state, error) {
		var s state
		err := p.env.Host.CallCompletingInto("interconnect.get", nil, &s)
		return s, err
	}, func(s state, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.state = &s
		if reset || p.draft == nil {
			draft := s.Settings
			p.draft = &draft
		}
	})
	core.Go(p.env, func() (general, error) {
		var g general
		err := p.env.Host.CallCompletingInto("interconnect.general.get", nil, &g)
		return g, err
	}, func(g general, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.general = &g
		if reset || p.draftGen == nil {
			draft := g
			p.draftGen = &draft
		}
	})
}

// readClipboard looks for a transfer code on the clipboard, when the
// setting allows, as the page opens or the window regains the focus. It
// offers the code; nothing connects until the user chooses.
func (p *page) readClipboard(c *ui.Context) {
	focuses := p.env.Focuses()
	if focuses == p.focuses {
		return
	}
	p.focuses = focuses
	text := c.Services().ReadClipboard()
	core.Go(p.env, func() (general, error) {
		var g general
		err := p.env.Host.CallCompletingInto("interconnect.general.get", nil, &g)
		return g, err
	}, func(g general, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		code := strings.TrimSpace(text)
		if !g.Clipboard || len(code) > 4096 {
			return
		}
		if kind := classify(code); kind != "" {
			p.clipboard = &clipboardCode{kind: kind, code: code}
		}
	})
}

func (p *page) dirty() (preferencesDirty, generalDirty bool) {
	if p.draft != nil && p.state != nil {
		a, _ := json.Marshal(p.draft)
		b, _ := json.Marshal(p.state.Settings)
		preferencesDirty = string(a) != string(b)
	}
	if p.draftGen != nil && p.general != nil {
		generalDirty = *p.draftGen != *p.general
	}
	return
}

func (p *page) undo() {
	if p.state != nil {
		draft := p.state.Settings
		p.draft = &draft
	}
	if p.general != nil {
		draft := *p.general
		p.draftGen = &draft
	}
	p.err = ""
}

// save writes each changed part on its own: a later failure leaves only
// its own draft unsaved, and saving again repeats nothing done.
func (p *page) save() {
	preferencesDirty, generalDirty := p.dirty()
	draft, draftGen := *p.draft, *p.draftGen
	p.busy, p.err = true, ""
	core.Do(p.env, func() error {
		if preferencesDirty {
			var saved preferences
			if err := p.env.Host.CallCompletingInto("interconnect.preferences.save", draft, &saved); err != nil {
				return err
			}
			p.env.Post(func() {
				if p.state != nil {
					p.state.Settings = saved
				}
				copy := saved
				p.draft = &copy
			})
		}
		if generalDirty {
			var saved general
			if err := p.env.Host.CallCompletingInto("interconnect.general.save", draftGen, &saved); err != nil {
				return err
			}
			p.env.Post(func() {
				p.general = &saved
				copy := saved
				p.draftGen = &copy
			})
		}
		return nil
	}, func(err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.env.Toast(i18n.T("common.saved"))
		p.reload(false)
	})
}

func (p *page) Close() {
	p.files.Close()
	p.store.Close()
	p.tunnel.Close()
	p.env.Activity.Report("interconnect", false, false, nil)
	p.env.Activity.Report("interconnect-general", false, false, nil)
}

func (p *page) View(c *ui.Context) {
	p.readClipboard(c)
	preferencesDirty, generalDirty := p.dirty()
	settingsDirty := preferencesDirty || generalDirty
	// Sharing goes on across pages; only unfinished input and running
	// operations are guarded.
	p.env.Activity.Report("interconnect", settingsDirty, p.busy, p.undo)
	p.files.Report()
	p.store.Report()
	p.tunnel.Report()
	p.settings.Report()
	// Explorer's "send with WinOne" prefills the file transfer.
	for _, intent := range p.env.TakeIntents("interconnect") {
		p.tab = tabFiles
		p.files.Apply(intent)
	}
	validPort := p.draft != nil && p.draft.Port == float64(int(p.draft.Port)) && p.draft.Port >= 0 && p.draft.Port <= 65535
	p.env.Rail.Set(
		core.Action{Label: tr("saveSettings"), Disabled: p.busy || !settingsDirty || p.draft == nil || p.draftGen == nil || !validPort, Busy: p.busy, Run: p.save},
		core.Action{Disabled: p.busy || !settingsDirty, Run: p.undo},
	)
	kit.Page(c, func() {
		if p.clipboard != nil {
			kit.Notice(c, kit.Info, "", tr("clipboardFound"), func() {
				if kit.Button(c, tr("useClipboard"), nil, false).Clicked() {
					intent := core.Intent{Requested: true, Paths: []string{}, Receive: true, Code: p.clipboard.code}
					if p.clipboard.kind == "store" {
						p.tab = tabStore
						p.store.Apply(intent)
					} else {
						p.tab = tabFiles
						p.files.Apply(intent)
					}
					p.clipboard = nil
				}
			})
		}
		kit.Tabs(c, &p.tab, []kit.Tab{
			{Label: tr("fileTransfer"), Icon: icons.Send},
			{Label: tr("store"), Icon: icons.Cloud},
			{Label: tr("tunnel"), Icon: icons.Network},
			{Label: tr("general"), Icon: icons.Settings},
		}, p.env.RequestChange)
		if p.err != "" {
			kit.Notice(c, kit.Failure, "", p.err, func() {
				if kit.Button(c, tr("retry"), nil, false).Disabled(p.busy).Clicked() {
					p.err = ""
					p.reload(false)
				}
			})
		}
		switch p.tab {
		case tabFiles:
			p.files.View(c, true, settingsDirty)
		case tabStore:
			p.store.View(c, true, false)
		case tabTunnel:
			p.tunnel.View(c, true)
		default:
			p.generalView(c)
		}
	})
}

// generalView is the sharing configuration and the general settings,
// saved together from the rail.
func (p *page) generalView(c *ui.Context) {
	kit.Group(c, icons.Network, tr("configuration"), tr("settingsForNextSend"), func() {
		if p.draft == nil {
			kit.Stack(c, "", "", func() { kit.Status(c, tr("loading")) })
			return
		}
		d := p.draft
		kit.Row(c, tr("network"), "", func() {
			options := []kit.SelectOption{{Value: "0.0.0.0", Label: tr("all")}, {Value: "127.0.0.1", Label: tr("local")}}
			if p.state != nil {
				for _, a := range p.state.Addresses {
					if a.IP != "127.0.0.1" {
						options = append(options, kit.SelectOption{Value: a.IP, Label: strings.TrimSpace(a.Name + " " + a.IP)})
					}
				}
			}
			_, trigger := kit.Select(c, &d.Bind, options)
			trigger.Label(tr("network")).Disabled(p.busy)
		})
		kit.Row(c, tr("port"), "", func() {
			kit.NumberField(c, &d.Port, 0, 65535, 1, "", tr("port")).Disabled(p.busy)
		})
		kit.Stack(c, tr("directory"), "", func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &d.DownloadDir).Grow(1).MinWidth(0).Label(tr("directory")).Disabled(p.busy)
				if kit.Button(c, tr("choose"), icons.FolderOpen, false).Disabled(p.busy).Clicked() {
					p.pickFolder(func(path string) { p.draft.DownloadDir = path })
				}
			})
		})
	})
	p.settings.View(c, p.draftGen, p.busy, p.pickFolder)
}

func (p *page) pickFolder(set func(string)) {
	p.busy, p.err = true, ""
	core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(tr("choose")) }, func(path string, err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
		} else if path != "" {
			set(path)
		}
	})
}

// menuState is Explorer's "send with WinOne" command.
type menuState struct {
	Enabled  bool   `json:"enabled"`
	Partial  bool   `json:"partial,omitempty"`
	Conflict string `json:"conflict,omitempty"`
}

// entryState is the floating receive window.
type entryState struct {
	Visible bool   `json:"visible"`
	Error   string `json:"error,omitempty"`
}

// generalPanel holds the settings that apply at once: the floating entry
// and Explorer's command. Its saved settings belong to the page's draft.
type generalPanel struct {
	env   *core.Env
	menu  *menuState
	entry *entryState
	busy  bool
	err   string
	read  bool
}

func (g *generalPanel) Report() {
	g.env.Activity.Report("interconnect-general", false, g.busy, nil)
}

func (g *generalPanel) load() {
	g.read = true
	type result struct {
		menu  menuState
		entry entryState
	}
	core.Go(g.env, func() (result, error) {
		var r result
		if err := g.env.Host.CallCompletingInto("interconnect.menu.get", nil, &r.menu); err != nil {
			return r, err
		}
		err := g.env.Host.CallCompletingInto("interconnect.entry.get", nil, &r.entry)
		return r, err
	}, func(r result, err error) {
		if err != nil {
			g.err = err.Error()
			return
		}
		g.menu, g.entry = &r.menu, &r.entry
	})
}

func (g *generalPanel) run(method string, params any, done func(json.RawMessage)) {
	g.busy, g.err = true, ""
	core.Go(g.env, func() (json.RawMessage, error) {
		var raw json.RawMessage
		err := g.env.Host.CallCompletingInto("interconnect."+method, params, &raw)
		return raw, err
	}, func(raw json.RawMessage, err error) {
		g.busy = false
		if err != nil {
			g.err = err.Error()
			return
		}
		done(raw)
	})
}

func (g *generalPanel) View(c *ui.Context, draft *general, disabled bool, pick func(func(string))) {
	if !g.read {
		g.load()
	}
	if g.err != "" {
		kit.Notice(c, kit.Failure, "", g.err, nil)
	}
	kit.Group(c, icons.Settings, tr("general"), "", func() {
		if draft != nil {
			kit.Stack(c, tr("defaultDownload"), "", func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					value := draft.DownloadDir
					ui.TextInput(c, &value).Grow(1).MinWidth(0).ReadOnly(true).Label(tr("defaultDownload"))
					if kit.Button(c, tr("choose"), icons.FolderOpen, false).Disabled(g.busy || disabled).Clicked() {
						pick(func(path string) { draft.DownloadDir = path })
					}
				})
			})
			kit.Toggle(c, &draft.Notify, tr("completionNotice"), "").Disabled(g.busy || disabled)
			kit.Toggle(c, &draft.Clipboard, tr("clipboardRecognition"), tr("clipboardHint")).Disabled(g.busy || disabled)
		}
		if g.entry != nil {
			visible := g.entry.Visible
			if kit.Toggle(c, &visible, tr("floatingEntry"), tr("floatingHint")).Disabled(g.busy).Changed() {
				method := "entry.hide"
				if visible {
					method = "entry.show"
				}
				g.run(method, nil, func(raw json.RawMessage) {
					var state entryState
					if json.Unmarshal(raw, &state) == nil {
						g.entry = &state
					}
				})
			}
			if g.entry.Error != "" {
				kit.Stack(c, "", "", func() { kit.InlineError(c, g.entry.Error) })
			}
		}
		if g.menu != nil {
			enabled := g.menu.Enabled
			if kit.Toggle(c, &enabled, tr("explorerEntry"), tr("explorerHint")).Disabled(g.busy || g.menu.Conflict != "").Changed() {
				g.run("menu.set", map[string]any{"enabled": enabled}, func(raw json.RawMessage) {
					var state menuState
					if json.Unmarshal(raw, &state) == nil {
						g.menu = &state
					}
				})
			}
			if g.menu.Conflict != "" {
				kit.Stack(c, "", "", func() { kit.InlineError(c, g.menu.Conflict) })
			}
			if g.menu.Partial {
				kit.Stack(c, "", "", func() { kit.InlineError(c, tr("menuPartial")) })
			}
		}
		kit.Stack(c, "", tr("backgroundHint"), func() {})
	})
}

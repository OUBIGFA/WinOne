package aiservices

import (
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// editor is the state of the provider the custom or accounts tab shows,
// beyond the draft: the name being typed, revealed keys, header rows, the
// picture's choices, the model list and the sign-in. React kept these in
// components keyed by the provider, so another provider starts afresh;
// the editor is made again when the provider (or its account type)
// changes.
type editor struct {
	p   *page
	key string
	id  string

	renaming     bool
	renameOrigin string
	renameFocus  bool

	gen          int // the page's savedGen the rows were read at
	fields       fields
	proxyProbes  map[string]*proxyProbePanel
	keyRows      map[string]*keyRow
	keyRests     keyRestPanel
	providerKeys providerKeysPanel
	plan         planPanel
	headers      []*headerRow
	avatar       avatarState
	models       *modelsPanel
	account      *accountPanel
	// Connection setup is secondary after a provider has models. New
	// providers open it immediately; switching views keeps the same draft.
	connectionOpen       bool
	setupFirst           bool
	optionsOpen          bool
	preferenceTab        int
	preferenceAccount    string
	newPreferenceAccount string
	newCapWindow         string
	claudeMode           claudeModePanel
	claudeModeDetails    bool
	endpointsOpen        bool
	serverCertificate    *serverCertificate
	certificateSource    string
	bulkKeysOpen         bool
	bulkKeysText         string
	bulkKeysBusy         bool
	bulkKeysError        string
	closed               bool
}

func newEditor(p *page, pr *provider, key string) *editor {
	e := &editor{p: p, key: key, keyRows: map[string]*keyRow{}, fields: fields{}}
	if pr == nil {
		return e
	}
	e.id = pr.ID
	e.connectionOpen = p.saved == nil || p.saved.Value.find(pr.ID) == nil || len(pr.available()) == 0 || (pr.AuthMode == "api-key" && strings.TrimSpace(pr.BaseURL) == "")
	e.setupFirst = e.connectionOpen
	e.renaming = pr.Name == ""
	e.renameFocus = e.renaming
	e.reset(pr)
	e.models = newModelsPanel(p, pr)
	if pr.AuthMode == "account" {
		e.account = newAccountPanel(p, pr)
	}
	return e
}

// reset rereads what the saved configuration replaced: typed header rows,
// revealed keys.
func (e *editor) reset(pr *provider) {
	e.keyRests.api.close()
	e.keyRests = keyRestPanel{}
	e.providerKeys.api.close()
	e.providerKeys = providerKeysPanel{}
	e.gen = e.p.savedGen
	for _, row := range e.keyRows {
		row.read++
	}
	e.keyRows = map[string]*keyRow{}
	e.headers = nil
	for _, row := range headerRows(pr.Headers) {
		row := row
		e.headers = append(e.headers, &row)
	}
}

func (e *editor) Close() {
	e.plan.read++
	if e.plan.auth != nil {
		e.plan.auth.Close()
	}
	e.keyRests.api.close()
	e.providerKeys.api.close()
	e.closed = true
	e.claudeMode.api.close()
	for _, row := range e.keyRows {
		row.read++
	}
	if e.account != nil {
		e.account.Close()
	}
	if e.models != nil {
		e.models.Close()
	}
}

// providersView is the custom or accounts tab: the toolbar choosing,
// naming and adding providers, and the chosen provider's settings.
func (p *page) providersView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	pr := p.current()
	key := ""
	if pr != nil {
		key = pr.ID + ":" + pr.Agent + ":" + pr.AgentBackend
	}
	if p.editor == nil || p.editor.key != key {
		if p.editor != nil {
			p.editor.Close()
		}
		p.editor = newEditor(p, pr, key)
	}
	e := p.editor
	if pr != nil && e.gen != p.savedGen {
		e.reset(pr)
	}
	accounts := p.tab == tabAccounts
	locked := p.locked()
	full := len(p.draft.Value.Providers) >= maxProviders
	addAccount := func() { p.agentSearch, p.choosingAgent = "", true }
	addPreset := func() {
		p.presetSearch, p.choosingPreset = "", true
		p.loadCatalog()
	}
	if len(p.sectionProviders()) == 0 {
		// An empty tab teaches its first step instead of a bare toolbar:
		// the easiest way in is the primary action.
		if accounts && p.catalog.busy {
			return
		}
		kit.Panel(c, func() {
			icon, title := icons.LayersPlus, "emptyCustom"
			hint := "ai.noServices"
			if accounts {
				icon, title, hint = icons.UserRound, "emptyAccounts", "ai.noAccounts"
			}
			kit.Empty(c, icon, homeT(title), i18n.T(hint), func() {
				if accounts {
					if kit.PrimaryButton(c, i18n.T("ai.addAccountProvider"), icons.Plus, false).
						Disabled(locked || full || len(p.catalog.agents) == 0).Clicked() {
						addAccount()
					}
					return
				}
				if kit.PrimaryButton(c, i18n.T("ai.addPreset"), icons.PlugZap, false).Disabled(locked || full).Clicked() {
					addPreset()
				}
				if kit.Button(c, homeT("addManually"), icons.Plus, false).Disabled(locked || full).Clicked() {
					p.add(nil)
				}
			})
		})
		return
	}
	ui.Column(c).FillWidth().Gap(20).Children(func() {
		var old *provider
		if pr != nil && p.saved != nil {
			old = p.saved.Value.find(pr.ID)
		}
		pal := kit.Colors(c)
		// Keep provider selection and its actions in one wrapping toolbar panel.
		// Grouping controls inside a shared Panel aligns them to the 16 DIP page
		// grid and centers every control on a single horizontal baseline.
		kit.Panel(c, func() {
			ui.Row(c).FillWidth().Padding(12, kit.RowPadX).GapX(12).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
				if pr != nil {
					ui.Column(c).Basis(220).Grow(1).MinWidth(0).Children(func() { e.nameField(c, pr, locked) })
					ui.Box(c).Width(1).Height(20).Background(pal.Hairline).Shrink(0)
					enabled := !pr.Off
					ui.Row(c).MinHeight(kit.ControlHeight).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
						kit.Help(c, i18n.T("aiExperience.enabled")).SingleLine()
						ui.Switch(c, &enabled).Label(completeT("providerEnabled")).Description(completeT("providerEnabledHint")).Disabled(locked)
					})
					pr.Off = !enabled
					ui.Box(c).Width(1).Height(20).Background(pal.Hairline).Shrink(0)
				}
				if !e.optionsOpen {
					if kit.GhostButton(c, i18n.T("aiBulkProviders.open"), icons.ListChecks, false).Disabled(locked).Clicked() {
						p.bulkProvidersOpen = true
					}
					if kit.Button(c, i18n.T("aiExperience.add"), icons.Plus, false).Disabled(locked || full || (accounts && len(p.catalog.agents) == 0)).Clicked() {
						if accounts {
							addAccount()
						} else {
							p.add(nil)
						}
					}
					if !accounts && kit.Button(c, i18n.T("aiExperience.presets"), icons.PlugZap, false).Disabled(locked || full).Clicked() {
						addPreset()
					}
				}
				if pr != nil {
					label, icon := "aiExperience.settings", icons.Settings2
					if e.optionsOpen {
						label, icon = "aiExperience.back", icons.ChevronLeft
					}
					if kit.Button(c, i18n.T(label), icon, false).Disabled(locked).Clicked() {
						e.optionsOpen = !e.optionsOpen
					}
				}
			})
		})
		if pr != nil {
			e.View(c, pr, old)
		}
	})
}

// nameField chooses the provider and renames it in one place: a select
// of the tab's providers, or the name being typed. Enter and leaving the
// field finish typing; Escape puts the name back.
func (e *editor) nameField(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	accountMode := pr.AuthMode == "account"
	label, nameLabel := "ai.service", "ai.serviceName"
	action := "ai.renameService"
	if accountMode {
		label, nameLabel, action = "ai.account", "ai.accountName", "ai.renameAccount"
	}
	if e.renaming {
		action = "ai.finishRename"
	}
	id := pr.ID
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if e.renaming {
			field := e.fields.get("name")
			if name, edited := field.sync(pr.Name); edited {
				p.change(id, func(t *provider) { t.Name = name })
			}
			input := kit.TextInput(c, &field.text).Grow(1).MinWidth(0).Label(i18n.T(nameLabel)).Disabled(locked).
				OnTextKey(func(k ui.TextKey) bool {
					switch k.Key {
					case ui.KeyEnter:
						e.renaming = false
						return true
					case ui.KeyEscape:
						if target := p.draft.Value.find(id); target != nil {
							target.Name = e.renameOrigin
						}
						field.text, field.shown = e.renameOrigin, e.renameOrigin
						e.renaming = false
						return true
					}
					return false
				})
			if e.renameFocus {
				input.Focus()
				e.renameFocus = false
			} else if !input.Focused() {
				// Leaving the field finishes typing, as its blur did.
				e.renaming = false
			}
		} else {
			var options []widgets.PictureOption
			for _, other := range p.sectionProviders() {
				name := other.Name
				if name == "" {
					name = i18n.T("ai.unnamed")
				}
				mark := other.mark()
				options = append(options, widgets.PictureOption{Value: other.ID, Label: name,
					Picture: func(c *ui.Context) { widgets.ProviderIcon(c, p.env, mark, 16) }})
			}
			chosen := pr.ID
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				if widgets.PictureSelect(c, &chosen, options, "", i18n.T(label), locked) {
					p.selected = chosen
				}
			})
		}
		b := kit.IconButton(c, map[bool]*ui.SVG{true: icons.Check, false: icons.Pencil}[e.renaming], i18n.T(action)).Disabled(locked)
		if b.Clicked() {
			if e.renaming {
				e.renaming = false
			} else {
				e.renameOrigin, e.renaming, e.renameFocus = pr.Name, true, true
			}
		}
	})
}

var httpAddress = regexp.MustCompile(`(?i)^http://`)

// View shows the provider's settings: its account or connection, its
// models and the panel plugins add for it.
func (e *editor) View(c *ui.Context, pr *provider, old *provider) {
	p := e.p
	if e.account != nil {
		e.account.prepare(pr)
	}
	if e.plan.auth != nil {
		e.plan.auth.report()
	}
	locked := p.locked()
	if e.optionsOpen {
		e.preferences(c, pr, locked)
		return
	}
	// Setup leads only while needed. Configured providers land on their
	// models; low-frequency connection fields no longer separate the two.
	// Keep the editor's order stable while typing. Re-evaluating an empty
	// address every frame moves its input when Select All/Delete is used,
	// drops focus, and prevents typing the replacement address.
	needsSetup := e.setupFirst
	if a := e.account; a != nil {
		needsSetup = needsSetup || old == nil || a.loading || len(a.accounts) == 0 || a.job.running() || a.err != "" || slices.ContainsFunc(a.accounts, func(ac account) bool { return ac.Lapsed != "" })
	}
	if needsSetup {
		e.connectionSection(c, pr, old, locked)
	}
	section := ui.Column(c).FillWidth().Gap(12).Label(i18n.T("ai.modelSettings")).Focusable()
	if p.focusModels {
		p.focusModels = false
		section.ScrollIntoView()
		section.Focus()
	}
	section.Children(func() { e.models.View(c, pr) })
	if !needsSetup {
		e.connectionSection(c, pr, old, locked)
	}
	remove := "ai.removeService"
	if pr.AuthMode == "account" {
		remove = "ai.removeAccount"
	}
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		if kit.GhostButton(c, i18n.T(remove), icons.Trash2, false).TextColor(c.Theme().TextMuted).Disabled(locked).Clicked() {
			p.remove(pr.ID)
		}
	})
}

// connectionSection keeps connection details in the existing provider
// draft. Sign-in remains available in the daily account view.
func (e *editor) connectionSection(c *ui.Context, pr *provider, old *provider, locked bool) {
	if pr.AuthMode == "api-key" {
		summary := i18n.T("aiExperience.ready")
		if strings.TrimSpace(pr.BaseURL) == "" {
			summary = i18n.T("aiExperience.setup")
		}
		kit.Disclosure(c, i18n.T("aiExperience.connection"), summary, &e.connectionOpen, func() {
			e.connectionRows(c, pr, locked)
		})
		return
	}
	if e.account != nil {
		e.account.View(c, pr, old)
	}
}

// connection is a custom connection's API, address and keys.
func (e *editor) connection(c *ui.Context, pr *provider, locked bool) {
	kit.Panel(c, func() { e.connectionRows(c, pr, locked) })
}

func (e *editor) connectionRows(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	preset := p.catalog.preset(pr.Preset)
	id := pr.ID
	// The sub-tab above names the connection, so its rows need no
	// heading of their own.
	// Filling the form from a vendor's example is the quickest start, so
	// it leads the panel as a row of its own, with what it reads.
	kit.Row(c, i18n.T("ai.importSetup"), i18n.T("ai.importSetupHint"), func() {
		if kit.Button(c, i18n.T("ai.importSetupAction"), icons.ClipboardPaste, false).Disabled(locked).Clicked() {
			p.importProviderSetup(c.Services().ReadClipboard(), pr, "")
		}
	})
	kit.Row(c, i18n.T("ai.protocol"), "", func() {
		protocol := pr.Protocol
		if protocol == "" {
			protocol = "chat"
		}
		changed, trigger := kit.Select(c, &protocol, []kit.SelectOption{
			{Value: "chat", Label: "OpenAI Chat Completions"},
			{Value: "responses", Label: "OpenAI Responses"},
			{Value: "anthropic", Label: "Anthropic Messages"},
			{Value: "gemini", Label: "Google Gemini"},
			{Value: "decide", Label: "System One Decide"},
		})
		trigger.Label(i18n.T("ai.protocol")).Disabled(locked)
		if changed {
			p.changeProtocol(pr, protocol)
		}
	})
	if preset != nil && len(preset.Regions) > 0 {
		kit.Row(c, i18n.T("ai.presetRegion"), "", func() {
			region := pr.PresetRegion
			if region == "" {
				region = preset.Regions[0].ID
			}
			var options []kit.SelectOption
			for _, r := range preset.Regions {
				options = append(options, kit.SelectOption{Value: r.ID, Label: r.Name})
			}
			changed, trigger := kit.Select(c, &region, options)
			trigger.Label(i18n.T("ai.presetRegion")).Disabled(locked)
			if changed {
				chosen := *preset
				p.change(id, func(t *provider) {
					protocol, base := presetConnection(chosen, region, t.Protocol)
					models := chosen.modelIDs(region)
					t.PresetRegion, t.Protocol, t.BaseURL, t.Models, t.AvailableModels = region, protocol, base, []string{}, &models
				})
			}
		})
	}
	kit.Stack(c, i18n.T("ai.address"), "", func() {
		placeholder := "https://api.example.com/v1"
		if preset != nil && preset.Endpoint != "" {
			placeholder = preset.Endpoint
		}
		field := e.fields.get("baseUrl")
		if address, edited := field.sync(pr.BaseURL); edited {
			p.change(id, func(t *provider) { t.BaseURL = address })
		}
		kit.TextInput(c, &field.text).FillWidth().Placeholder(placeholder).Label(i18n.T("ai.address")).Disabled(locked).
			OnPaste(func(text string) bool {
				target := p.draft.Value.find(id)
				return target != nil && p.importCurl(text, target, "")
			})
		if httpAddress.MatchString(strings.TrimSpace(pr.BaseURL)) {
			kit.Help(c, i18n.T("ai.httpAddressHint"))
		}
	})
	e.keys(c, pr, locked)
	e.limitRest(c, pr, locked)
	e.endpointPreferences(c, pr, locked)
	if p.curl != nil && p.curl.provider == id {
		kit.Stack(c, "", "", func() {
			for _, message := range p.curl.messages {
				kit.Help(c, message).Role(ui.RoleStatus)
			}
		})
	}
	if preset != nil && preset.Kind == "local" {
		kit.Stack(c, "", i18n.T("ai.localServerHint"), func() {})
	}
}

// limitRest is the switch resting a provider's keys on quota and rate
// limits.
func (e *editor) limitRest(c *ui.Context, pr *provider, locked bool) {
	rest := !pr.LimitRestOff
	if kit.Toggle(c, &rest, i18n.T("ai.limitRest"), i18n.T("ai.limitRestHint")).Disabled(locked).Changed() {
		e.p.change(pr.ID, func(t *provider) { t.LimitRestOff = !rest })
	}
}

// keyRow is one key slot's state in the editor: the text the field shows,
// and a revealed saved key, which never becomes an edit.
type keyRow struct {
	text, shown string
	visible     bool
	saved       string
	reading     bool
	read        int // drops a reveal that a hide, reset or edit replaced
	err         string
}

// keys edits the provider's key slots: each key's field, reveal, switch
// and removal, and how several keys share requests.
func (e *editor) keys(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	id := pr.ID
	slots := pr.slots()
	edits := p.keys[id]
	kit.Stack(c, "", "", func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			kit.Label(c, i18n.T("ai.key"))
			if kit.IconButton(c, icons.Plus, i18n.T("ai.addKey")).Disabled(locked).Clicked() {
				slot := keySlot{ID: newID(), Enabled: true}
				p.change(id, func(t *provider) {
					next := append(append([]keySlot{}, t.slots()...), slot)
					t.Keys = &next
				})
			}
		})
		for i, slot := range slots {
			e.keyRow(c.Key(slot.ID), pr, slot, i, len(slots), edits, locked)
		}
		e.bulkKeys(c, pr, locked)
		ui.Row(c).FillWidth().Children(func() { e.keyRestControls(c, pr, locked) })
	})
}

func (e *editor) keyRow(c *ui.Context, pr *provider, slot keySlot, index, count int, edits map[string]string, locked bool) {
	p := e.p
	id, slotID := pr.ID, slot.ID
	row := e.keyRows[slotID]
	if row == nil {
		row = &keyRow{}
		e.keyRows[slotID] = row
	}
	edit, edited := edits[slotID]
	// What the user typed since the last frame is an edit; otherwise the
	// field shows the edit, or the revealed key, or nothing.
	if row.text != row.shown {
		row.shown = row.text
		row.read++
		row.reading, row.saved, row.err = false, "", ""
		text := row.text
		p.change(id, func(t *provider) {
			materialized := append([]keySlot{}, t.slots()...)
			t.Keys = &materialized
		})
		if text == "" {
			p.changeKey(id, slotID, nil)
		} else {
			p.changeKey(id, slotID, &text)
		}
		edit, edited = text, text != ""
	} else {
		want := ""
		switch {
		case edited:
			want = edit
		case row.visible:
			want = row.saved
		}
		row.text, row.shown = want, want
	}
	label := i18n.T("ai.key")
	if count > 1 {
		label = i18n.T("ai.keyNumber", "index", index+1)
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if count > 1 {
				ui.Text(c, kit.Itoa(index+1)).Width(16).TextAlign(ui.End).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).Shrink(0)
			}
			placeholder := "ai.keyOptional"
			if slot.HasKey {
				placeholder = "ai.keySaved"
			}
			input := kit.TextInput(c, &row.text).Grow(1).MinWidth(0).Label(label).Placeholder(i18n.T(placeholder)).
				Disabled(locked || row.reading).OnPaste(func(text string) bool {
				target := p.draft.Value.find(id)
				return target != nil && p.importCurl(text, target, slotID)
			})
			if !row.visible {
				input.Password()
			}
			if !slot.Enabled {
				input.Opacity(0.6)
			}
			action := "ai.showKey"
			eye := icons.Eye
			if row.visible {
				action, eye = "ai.hideKey", icons.EyeOff
			}
			if kit.IconButton(c, eye, i18n.T(action, "name", label)).Disabled(locked || (!edited && !slot.HasKey) || row.reading).Clicked() {
				e.toggleKey(id, slotID, row, edited)
			}
			enabled := slot.Enabled
			if ui.Switch(c, &enabled).Label(i18n.T("ai.enableKey", "index", index+1)).Description(i18n.T("ai.enableKey", "index", index+1)).
				Disabled(locked).Changed() {
				p.change(id, func(t *provider) {
					next := append([]keySlot{}, t.slots()...)
					for i := range next {
						if next[i].ID == slotID {
							next[i].Enabled = enabled
						}
					}
					t.Keys = &next
				})
			}
			if kit.IconButton(c, icons.Trash2, i18n.T("ai.removeKey", "index", index+1)).Disabled(locked).Clicked() {
				row.read++
				delete(e.keyRows, slotID)
				p.change(id, func(t *provider) {
					var next []keySlot
					for _, s := range t.slots() {
						if s.ID != slotID {
							next = append(next, s)
						}
					}
					if next == nil {
						next = []keySlot{}
					}
					t.Keys = &next
				})
				p.changeKey(id, slotID, nil)
			}
		})
		kit.InlineError(c, row.err)
		e.keyRestRow(c, pr, slotID, locked || edited || !slot.Enabled)
	})
}

// toggleKey shows or hides a key. A typed key shows without the Host; a
// saved one is read, and a reveal that a hide, reset or edit overtook is
// dropped.
func (e *editor) toggleKey(provider, slot string, row *keyRow, edited bool) {
	row.err = ""
	if row.visible {
		row.read++
		row.visible, row.saved = false, ""
		return
	}
	if edited {
		row.visible = true
		return
	}
	row.read++
	owner := row.read
	row.reading = true
	core.Go(e.p.env, func() (string, error) {
		var result struct {
			Key string `json:"key"`
		}
		err := e.p.env.Host.CallCompletingInto("capture.ai.keys.reveal", map[string]string{"provider": provider, "id": slot}, &result)
		return result.Key, err
	}, func(key string, err error) {
		if row.read != owner {
			return
		}
		row.reading = false
		if err != nil {
			row.err = err.Error()
			return
		}
		row.saved, row.visible = key, true
	})
}

// headersView edits the request headers as rows, Magpie's ordered
// name/value/JSON editor; the draft keeps only their serialization.
func (e *editor) headersView(c *ui.Context, pr *provider, locked bool) {
	var hints []string
	if preset := e.p.catalog.preset(pr.Preset); preset != nil {
		hints = preset.HeaderHints
	}
	if requestHeaderRows(c, &e.headers, 64, locked, hints) {
		rows := make([]headerRow, len(e.headers))
		for i, row := range e.headers {
			rows[i] = *row
		}
		e.p.change(pr.ID, func(t *provider) { t.Headers = headersOf(rows) })
	}
}

// Provider and model request headers use the same name/value/JSON controls.
func requestHeaderRows(c *ui.Context, rows *[]*headerRow, limit int, locked bool, hints ...[]string) bool {
	changed := false
	kit.Group(c, icons.Braces, i18n.T("ai.headers"), i18n.T("ai.headersHint"), func() {
		kit.Stack(c, "", "", func() {
			remove := -1
			for i, row := range *rows {
				row := row
				ui.Column(c.Key(i)).FillWidth().Gap(8).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						name := kit.TextInput(c, &row.Name).Placeholder("Header-Name").Label(i18n.T("ai.headerName") + " " + kit.Itoa(i+1)).Disabled(locked)
						if row.JSON {
							name.Grow(1).MinWidth(0)
						} else {
							name.Grow(1).Basis(0).MinWidth(0)
						}
						changed = changed || name.Changed()
						if !row.JSON {
							value := kit.TextInput(c, &row.Value).Grow(2).Basis(0).MinWidth(0).Placeholder(i18n.T("ai.headerValue")).
								Label(i18n.T("ai.headerValue") + " " + kit.Itoa(i+1)).Disabled(locked)
							changed = changed || value.Changed()
						}
						mode := "ai.headerJSON"
						if row.JSON {
							mode = "ai.headerSingleLine"
						}
						if kit.IconButton(c, icons.Braces, i18n.T(mode)).Disabled(locked).Clicked() {
							if row.JSON {
								row.Value = minifyJSON(row.Value)
							} else {
								row.Value = prettyJSON(row.Value)
							}
							row.JSON = !row.JSON
							changed = true
						}
						if kit.IconButton(c, icons.X, i18n.T("ai.removeHeader")+" "+kit.Itoa(i+1)).Disabled(locked).Clicked() {
							remove = i
						}
					})
					if row.JSON {
						area := ui.TextArea(c, &row.Value).FillWidth().MinHeight(96).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).
							Label(i18n.T("ai.headerValue") + " " + kit.Itoa(i+1)).Disabled(locked)
						changed = changed || area.Changed()
					}
				})
			}
			if remove >= 0 {
				(*rows) = append((*rows)[:remove], (*rows)[remove+1:]...)
				changed = true
			}
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				if kit.GhostButton(c, i18n.T("ai.addHeader"), icons.Plus, false).Disabled(locked || len((*rows)) >= limit).Clicked() {
					(*rows) = append((*rows), &headerRow{})
					changed = true
				}
				selected := ""
				choices := []kit.SelectOption{{Value: "", Label: i18n.T("aiSync.headerPresets")}}
				for _, preset := range headerPresets {
					choices = append(choices, kit.SelectOption{Value: preset.Name, Label: preset.Name})
				}
				picked, trigger := kit.Select(c, &selected, choices)
				trigger.Label(i18n.T("aiSync.headerPresets")).Disabled(locked)
				if picked && selected != "" {
					for _, preset := range headerPresets {
						if preset.Name == selected {
							applyHeaderPreset(rows, preset)
							changed = true
							break
						}
					}
				}
				for _, group := range hints {
					for _, hint := range group {
						found := false
						for _, row := range *rows {
							if strings.EqualFold(strings.TrimSpace(row.Name), hint) {
								found = true
								break
							}
						}
						if !found && kit.GhostButton(c, "+ "+hint, nil, false).Disabled(locked || len(*rows) >= limit).Clicked() {
							*rows = append(*rows, &headerRow{Name: hint})
							changed = true
						}
					}
				}
			})
		})
	})
	return changed
}

// named is a provider's or account type's public identity, all a plugin
// is told of it.
type named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

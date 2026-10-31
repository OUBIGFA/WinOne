package aiservices

import (
	"regexp"
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

	gen     int // the page's savedGen the rows were read at
	fields  fields
	keyRows map[string]*keyRow
	headers []*headerRow
	avatar  avatarState
	models  *modelsPanel
	account *accountPanel
}

func newEditor(p *page, pr *provider, key string) *editor {
	e := &editor{p: p, key: key, keyRows: map[string]*keyRow{}, fields: fields{}}
	if pr == nil {
		return e
	}
	e.id = pr.ID
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
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		toolbar := ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.End)
		var old *provider
		if pr != nil && p.saved != nil {
			old = p.saved.Value.find(pr.ID)
		}
		menu := p.extensions.providerMenu(old, p.catalog)
		if menu != nil && !locked {
			// Right-clicking the toolbar opens the plugins' actions, as
			// React's context menu shortcut did.
			toolbar.ContextMenu(func(m *ui.Menu) { menu(m) })
		}
		toolbar.Children(func() {
			if pr != nil {
				ui.Column(c).Basis(240).Grow(1).MinWidth(0).Children(func() { e.nameField(c, pr, locked) })
			}
			if menu != nil {
				b := kit.IconButton(c, icons.Blocks, i18n.T("aiPlugins.actions")).Disabled(locked)
				b.Menu(func(m *ui.Menu) { menu(m) })
			}
			label := "ai.addCustomProvider"
			if accounts {
				label = "ai.addAccountProvider"
			}
			full := len(p.draft.Value.Providers) >= maxProviders
			if kit.Button(c, i18n.T(label), icons.Plus, false).Disabled(locked || full || (accounts && len(p.catalog.agents) == 0)).Clicked() {
				if accounts {
					p.agentSearch, p.choosingAgent = "", true
				} else {
					p.add(nil)
				}
			}
			if !accounts && kit.Button(c, i18n.T("ai.addPreset"), icons.PlugZap, false).Disabled(locked || full).Clicked() {
				p.presetSearch, p.choosingPreset = "", true
				p.loadCatalog()
			}
		})
		if len(p.sectionProviders()) == 0 && !(accounts && p.catalog.busy) {
			hint := "ai.noServices"
			if accounts {
				hint = "ai.noAccounts"
			}
			kit.Help(c, i18n.T(hint))
		}
		if pr != nil {
			e.View(c, pr, old)
		}
		if accounts {
			p.extensions.Slot(c, "agents", nil, locked)
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
	ui.Field(c, i18n.T(label), func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if e.renaming {
				field := e.fields.get("name")
				if name, edited := field.sync(pr.Name); edited {
					p.change(id, func(t *provider) { t.Name = name })
				}
				input := ui.TextInput(c, &field.text).Grow(1).MinWidth(0).Label(i18n.T(nameLabel)).Disabled(locked).
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
	})
}

var httpAddress = regexp.MustCompile(`(?i)^http://`)

// View shows the provider's settings: its account or connection, its
// models and the panel plugins add for it.
func (e *editor) View(c *ui.Context, pr *provider, old *provider) {
	p := e.p
	locked := p.locked()
	if pr.AuthMode == "account" && e.account != nil {
		e.account.View(c, pr, old)
	}
	if pr.AuthMode == "api-key" {
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			ui.Column(c).Disabled(p.navigationLocked()).Children(func() {
				kit.SubTabs(c, &p.providerTab, []string{i18n.T("ai.connection"), i18n.T("ai.advanced")}, p.env.RequestChange)
			})
			if p.providerTab == providerConnection {
				e.connection(c, pr, locked)
			} else {
				e.advanced(c, pr, locked)
			}
		})
	}
	section := ui.Column(c).FillWidth().Gap(12).Label(i18n.T("ai.modelSettings")).Focusable()
	if p.focusModels {
		p.focusModels = false
		section.ScrollIntoView()
		section.Focus()
	}
	section.Children(func() {
		kit.Panel(c, func() {
			rest := !pr.LimitRestOff
			if kit.Toggle(c, &rest, i18n.T("ai.limitRest"), i18n.T("ai.limitRestHint")).Disabled(locked).Changed() {
				p.change(pr.ID, func(t *provider) { t.LimitRestOff = !rest })
			}
		})
		e.models.View(c, pr)
	})
	if old != nil {
		p.extensions.Slot(c, "provider", &named{ID: old.ID, Name: old.Name}, locked)
	}
	ui.Row(c).Children(func() {
		if kit.GhostButton(c, i18n.T("ai.removeService"), icons.Trash2, false).Disabled(locked).Clicked() {
			p.remove(pr.ID)
		}
	})
}

// connection is a custom connection's API, address and keys.
func (e *editor) connection(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	preset := p.catalog.preset(pr.Preset)
	id := pr.ID
	kit.Group(c, icons.PlugZap, i18n.T("ai.connection"), "", func() {
		kit.Row(c, i18n.T("ai.protocol"), "", func() {
			protocol := pr.Protocol
			if protocol == "" {
				protocol = "chat"
			}
			changed, trigger := kit.Select(c, &protocol, []kit.SelectOption{
				{Value: "chat", Label: "OpenAI Chat Completions"},
				{Value: "responses", Label: "OpenAI Responses"},
				{Value: "anthropic", Label: "Anthropic Messages"},
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
						models := append([]string{}, chosen.Models...)
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
			ui.TextInput(c, &field.text).FillWidth().Placeholder(placeholder).Label(i18n.T("ai.address")).Disabled(locked).
				OnPaste(func(text string) bool {
					target := p.draft.Value.find(id)
					return target != nil && p.importCurl(text, target, "")
				})
			if httpAddress.MatchString(strings.TrimSpace(pr.BaseURL)) {
				kit.Help(c, i18n.T("ai.httpAddressHint"))
			}
		})
		e.keys(c, pr, locked)
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
	})
}

// advanced is a custom connection's picture, deferred tools and headers.
func (e *editor) advanced(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	e.avatarView(c, pr, locked)
	kit.Panel(c, func() {
		tools := pr.AdditionalTools
		if kit.Toggle(c, &tools, i18n.T("ai.additionalTools"), i18n.T("ai.additionalToolsHint")).Disabled(locked).Changed() {
			p.change(pr.ID, func(t *provider) { t.AdditionalTools = tools })
		}
	})
	e.headersView(c, pr, locked)
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
			if kit.IconButton(c, icons.Plus, i18n.T("ai.addKey")).Disabled(locked || len(slots) >= 16).Clicked() {
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
		if len(slots) > 1 {
			e.policy(c, pr, locked)
		}
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
			input := ui.TextInput(c, &row.text).Grow(1).MinWidth(0).Label(label).Placeholder(i18n.T(placeholder)).
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
			if ui.Switch(c, &enabled).Label(i18n.T("ai.enableKey", "index", index+1)).Tooltip(i18n.T("ai.enableKey", "index", index+1)).
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

// policy is how several keys share requests: in turns or in order, and
// whether a failure moves on to the next key.
func (e *editor) policy(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	id := pr.ID
	policy := pr.policy()
	set := func(edit func(*keyPolicy)) {
		p.change(id, func(t *provider) {
			next := t.policy()
			edit(&next)
			t.KeyPolicy = &next
		})
	}
	ui.Column(c).FillWidth().Gap(12).Padding(8, 0, 0, 0).Children(func() {
		ui.Field(c, i18n.T("ai.keyStrategy"), func() {
			strategy := policy.Strategy
			ui.Column(c).Disabled(locked).Children(func() {
				if kit.Segmented(c, &strategy, []kit.SelectOption{
					{Value: "round-robin", Label: i18n.T("ai.roundRobinKeys")},
					{Value: "ordered", Label: i18n.T("ai.orderedKeys")},
				}, i18n.T("ai.keyStrategy")) {
					set(func(k *keyPolicy) { k.Strategy = strategy })
				}
			})
		})
		failover := policy.Failover
		if ui.Checkbox(c, &failover, i18n.T("ai.keyFailover")).Disabled(locked).Changed() {
			set(func(k *keyPolicy) { k.Failover = failover })
		}
		ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
			ui.Field(c, i18n.T("ai.keyMaxAttempts"), func() {
				ui.Column(c).Disabled(locked || !policy.Failover).Children(func() {
					if v, changed := numberText(c, &policy.MaxAttempts, 1, 16, true, "", i18n.T("ai.keyMaxAttempts")); changed && v != nil {
						set(func(k *keyPolicy) { k.MaxAttempts = *v })
					}
				})
			}).Basis(200).Grow(1)
			ui.Field(c, i18n.T("ai.keyCooldown"), func() {
				ui.Column(c).Disabled(locked).Children(func() {
					if v, changed := numberText(c, &policy.CooldownSeconds, 1, 3600, true, "", i18n.T("ai.keyCooldown")); changed && v != nil {
						set(func(k *keyPolicy) { k.CooldownSeconds = *v })
					}
				})
			}).Basis(200).Grow(1)
		})
		kit.Help(c, i18n.T("ai.keyFailoverHint"))
	})
}

// headersView edits the request headers as rows, Magpie's ordered
// name/value/JSON editor; the draft keeps only their serialization.
func (e *editor) headersView(c *ui.Context, pr *provider, locked bool) {
	p := e.p
	id := pr.ID
	changed := false
	kit.Group(c, icons.Braces, i18n.T("ai.headers"), i18n.T("ai.headersHint"), func() {
		kit.Stack(c, "", "", func() {
			remove := -1
			for i, row := range e.headers {
				row := row
				ui.Column(c.Key(i)).FillWidth().Gap(8).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						name := ui.TextInput(c, &row.Name).Placeholder("Header-Name").Label(i18n.T("ai.headerName") + " " + kit.Itoa(i+1)).Disabled(locked)
						if row.JSON {
							name.Grow(1).MinWidth(0)
						} else {
							name.Grow(1).Basis(0).MinWidth(0)
						}
						changed = changed || name.Changed()
						if !row.JSON {
							value := ui.TextInput(c, &row.Value).Grow(2).Basis(0).MinWidth(0).Placeholder(i18n.T("ai.headerValue")).
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
				e.headers = append(e.headers[:remove], e.headers[remove+1:]...)
				changed = true
			}
			ui.Row(c).Children(func() {
				if kit.GhostButton(c, i18n.T("ai.addHeader"), icons.Plus, false).Disabled(locked || len(e.headers) >= 64).Clicked() {
					e.headers = append(e.headers, &headerRow{})
					changed = true
				}
			})
		})
	})
	if changed {
		rows := make([]headerRow, len(e.headers))
		for i, row := range e.headers {
			rows[i] = *row
		}
		headers := headersOf(rows)
		p.change(id, func(t *provider) { t.Headers = headers })
	}
}

// named is a provider's or account type's public identity, all a plugin
// is told of it.
type named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

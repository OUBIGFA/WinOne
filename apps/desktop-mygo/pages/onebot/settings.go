package onebot

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type settingsPane interface {
	View(*ui.Context)
	Close()
}

func paneDirty(p settingsPane) bool {
	if p, ok := p.(interface{ Dirty() bool }); ok {
		return p.Dirty()
	}
	return false
}
func paneBusy(p settingsPane) bool {
	if p, ok := p.(interface{ Busy() bool }); ok {
		return p.Busy()
	}
	return false
}
func discardPane(p settingsPane) {
	if p, ok := p.(interface{ Discard() }); ok {
		p.Discard()
	}
}

type profileSettings struct {
	parent                *page
	botID, section        string
	baseline, draft       Profile
	panes                 map[string]settingsPane
	closed, saving, again bool
	error                 string
	cancelSave            func()
	defaultModels         bool
	chain, chainBaseline  []ModelRoute
	portrait              *portraitDialog
}

func newProfileSettings(parent *page, bot Profile, section string) *profileSettings {
	if section == "" {
		section = "home"
	}
	return &profileSettings{parent: parent, botID: bot.ID, section: section, baseline: copyValue(bot), draft: copyValue(bot), panes: map[string]settingsPane{}}
}
func (p *page) openDefaultModels() {
	p.env.RequestChange(func() {
		// Replacing the drawer ends its child lifetimes (including a voice
		// preview), just as opening another profile's settings already does.
		if p.settings != nil {
			p.settings.Close()
		}
		p.settings = &profileSettings{parent: p, section: "model", defaultModels: true, chain: copyValue(p.defaultChain), chainBaseline: copyValue(p.defaultChain), panes: map[string]settingsPane{}}
	})
}
func (d *profileSettings) Dirty() bool {
	if d.defaultModels {
		return !sameJSON(d.chain, d.chainBaseline)
	}
	return len(profilePatch(d.baseline, d.draft)) > 1
}
func (d *profileSettings) reportActivity() {
	d.parent.env.Activity.Report("onebot-profile", d.Dirty(), d.saving, d.Discard)
	for name, pane := range d.panes {
		current := pane
		d.parent.env.Activity.Report("onebot-settings-"+name, paneDirty(pane), paneBusy(pane), func() { discardPane(current) })
	}
}
func (d *profileSettings) Close() {
	if d.closed {
		return
	}
	d.closed = true
	if d.cancelSave != nil {
		d.cancelSave()
	}
	if d.portrait != nil {
		d.portrait.Close()
	}
	for name, pane := range d.panes {
		pane.Close()
		d.parent.env.Activity.Report("onebot-settings-"+name, false, false, nil)
	}
	d.parent.env.Activity.Report("onebot-profile", false, false, nil)
}
func (d *profileSettings) Discard() {
	if d.cancelSave != nil {
		d.cancelSave()
		d.cancelSave = nil
	}
	d.draft = copyValue(d.baseline)
	d.chain = copyValue(d.chainBaseline)
	d.error = ""
	for _, pane := range d.panes {
		discardPane(pane)
	}
}
func (d *profileSettings) changed() {
	if d.closed {
		return
	}
	if d.cancelSave != nil {
		d.cancelSave()
	}
	// Native IME marked text is not committed to these strings. Only actual
	// field changes schedule autosave, and a late receipt keeps newer typing.
	d.cancelSave = d.parent.after(650*time.Millisecond, func() { d.cancelSave = nil; d.Save() })
}
func (d *profileSettings) reconcile(bot Profile) {
	if d.saving || d.closed {
		return
	}
	if !d.Dirty() {
		d.draft = copyValue(bot)
		d.baseline = copyValue(bot)
		return
	}
	d.draft = rebaseProfile(d.draft, d.baseline, bot)
	d.baseline = copyValue(bot)
}
func (d *profileSettings) Save() {
	if d.closed || !d.Dirty() {
		return
	}
	if d.saving {
		d.again = true
		return
	}
	if !d.defaultModels && strings.TrimSpace(d.draft.Name) == "" {
		d.error = nt("nameRequired")
		return
	}
	d.saving = true
	d.error = ""
	if d.defaultModels {
		chain := copyValue(d.chain)
		oneBotCall(d.parent, "modelChain.set", map[string]any{"modelChain": chain}, func(value struct {
			ModelChain   []ModelRoute `json:"modelChain"`
			IsCustomized bool         `json:"isCustomized"`
		}, err error) {
			if d.closed {
				return
			}
			d.saving = false
			if err != nil {
				d.error = err.Error()
				return
			}
			d.chainBaseline = chain
			d.parent.defaultChain, d.parent.chainCustomized = value.ModelChain, value.IsCustomized
			d.finishSave()
		})
		return
	}
	captured := copyValue(d.draft)
	patch := profilePatch(d.baseline, captured)
	oneBotCall(d.parent, "profiles.update", map[string]any{"input": patch}, func(saved Profile, err error) {
		if d.closed {
			return
		}
		d.saving = false
		if err != nil {
			d.error = err.Error()
			d.again = false
			return
		}
		// Normalize only the fields whose captured values are still current.
		// The whole newer buffer stays local; the next patch sends its delta.
		d.draft = rebaseProfile(d.draft, captured, saved)
		d.baseline = copyValue(saved)
		d.parent.mergeProfile(saved)
		d.parent.avatars.load(d.parent, []Profile{saved})
		d.finishSave()
	})
}
func (d *profileSettings) finishSave() {
	if d.again {
		d.again = false
		d.Save()
	}
}
func (d *profileSettings) invalidated(kind, botID string) {
	if botID != "" && botID != d.botID {
		return
	}
	for _, pane := range d.panes {
		if refreshing, ok := pane.(interface{ Invalidate(string) }); ok {
			refreshing.Invalidate(kind)
		}
	}
}
func (d *profileSettings) requestSection(section string) {
	d.parent.env.RequestChange(func() { d.section = section })
}
func (d *profileSettings) pane(name string) settingsPane {
	if pane := d.panes[name]; pane != nil {
		return pane
	}
	var pane settingsPane
	switch name {
	case "memory":
		pane = newMemoryPane(d)
	case "capabilities":
		pane = newCapabilitiesPane(d)
	case "projects":
		pane = newProjectsPane(d)
	case "routines":
		pane = newRoutinesPane(d)
	case "channels":
		pane = newChannelsPane(d)
	case "voice":
		pane = newVoiceSettingsPane(d)
	case "microphone":
		pane = newMicrophonePane(d)
	case "history":
		pane = newHistoryPane(d)
	case "todos":
		pane = newTodosPane(d)
	case "glance":
		pane = newGlancePane(d)
	}
	if pane != nil {
		d.panes[name] = pane
	}
	return pane
}
func (d *profileSettings) View(c *ui.Context) {
	if d.closed {
		return
	}
	open := true
	title := nt("settings")
	if d.defaultModels {
		title = nt("defaultModels")
	}
	kit.Dialog(c, &open, kit.DialogOptions{Title: title, Width: 720, Cancel: nt("close"), Error: d.error, Busy: false, Header: func() {
		if !d.defaultModels {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if d.section != "home" && kit.IconButton(c, icons.ArrowLeft, nt("back")).Clicked() {
					d.requestSection("home")
				}
				d.parent.avatars.View(c, d.draft, 36)
				ui.Text(c, d.draft.Name).FontWeight(500).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if d.saving {
					kit.Loader(c)
				} else if d.Dirty() {
					kit.Caption(c, nt("unsaved"))
				}
			})
		}
	}, Actions: func() {
		if kit.Button(c, nt("undo"), icons.Undo2, false).Disabled(!d.Dirty() || d.saving).Clicked() {
			d.Discard()
		}
		if kit.PrimaryButton(c, nt("save"), icons.Save, d.saving).Disabled(!d.Dirty() || d.saving).Clicked() {
			d.Save()
		}
	}, Body: func() {
		if d.defaultModels {
			// This is a global conversation appearance preference, not a model
			// route field; keep it in the actually rendered default-model page.
			if kit.Toggle(c, &d.parent.prefs.ShowReplyTarget, bt("bots.globalSettings.showReplyTarget"), bt("bots.globalSettings.showReplyTargetHint")).Changed() {
				d.parent.savePreferences()
			}
			d.chainFields(c, &d.chain, func() { d.changed() })
			if kit.Button(c, bt("bots.model.restoreDefault"), icons.RotateCcw, false).Disabled(d.saving).Clicked() {
				d.restoreDefaultModels()
			}
			return
		}
		switch d.section {
		case "home":
			d.home(c)
		case "profile":
			d.profileFields(c)
		case "personality":
			d.textField(c, bt("bots.profile.personality"), &d.draft.IdentitySource, 240)
		case "model":
			d.modelFields(c)
		case "advanced":
			d.advanced(c)
		default:
			if pane := d.pane(d.section); pane != nil {
				pane.View(c)
			}
		}
	}})
	if !open {
		d.parent.env.RequestChange(func() {
			d.Close()
			if d.parent.settings == d {
				d.parent.settings = nil
			}
		})
	}
	if d.portrait != nil {
		d.portrait.View(c)
	}
}
func (d *profileSettings) home(c *ui.Context) {
	kit.Help(c, d.draft.Description)
	if invitation := d.draft.Invitation; invitation != nil && invitation.Stage != "ready" {
		kit.Status(c, nt("invitationStage", "stage", invitation.Stage))
		if invitation.Error != "" {
			kit.InlineError(c, invitation.Error)
		}
		if kit.Button(c, nt("retryInvitation"), icons.RefreshCw, false).Clicked() {
			d.parent.mutate("profiles.update", map[string]any{"input": map[string]any{"id": d.botID, "retryInvitation": true}}, nil)
		}
	}
	for _, item := range []struct {
		id, label string
		icon      *ui.SVG
	}{
		{"profile", bt("bots.profile.title"), icons.UserRound}, {"personality", bt("bots.profile.personality"), icons.MessageCircle},
		{"memory", bt("bots.memory.title"), icons.BookOpen}, {"projects", bt("bots.projects.title"), icons.FolderOpen},
		{"model", bt("bots.settingsTabs.model"), icons.Sparkles}, {"voice", bt("bots.voiceMode.settingsTitle"), icons.Volume2}, {"microphone", nt("microphone"), icons.Mic},
		{"capabilities", bt("bots.capabilities.title"), icons.Zap}, {"channels", bt("bots.channels.title"), icons.Plug},
		{"routines", bt("routines.title"), icons.Timer}, {"history", bt("bots.historySearch.title"), icons.History},
		{"todos", nt("todos"), icons.ListTodo}, {"glance", nt("glance"), icons.Activity},
		{"advanced", bt("bots.homeFolder.title"), icons.Settings2},
	} {
		button := kit.GhostButton(c, item.label, item.icon, false).FillWidth().Justify(ui.Start).MinWidth(0)
		if button.Clicked() {
			d.requestSection(item.id)
		}
	}
}
func (d *profileSettings) textField(c *ui.Context, label string, value *string, height float32) {
	ui.Field(c, label, func() {
		if ui.TextArea(c, value).FillWidth().MinWidth(0).Height(height).Label(label).Changed() {
			d.changed()
		}
	})
}
func (d *profileSettings) profileFields(c *ui.Context) {
	ui.Field(c, nt("name"), func() {
		if ui.TextInput(c, &d.draft.Name).FillWidth().Label(nt("name")).Changed() {
			d.changed()
		}
	})
	d.textField(c, bt("bots.profile.summary"), &d.draft.Description, 120)
	button := kit.Button(c, bt("bots.profile.changeAvatar"), icons.Camera, false)
	if button.Clicked() {
		d.choosePortrait()
	}
	widgets.DropArea(c, d.parent.env, button, widgets.DropFiles, false, d.saving, func(paths []string, err error) {
		if err != nil {
			d.error = err.Error()
		} else if len(paths) > 0 {
			d.portraitSource(paths[0])
		}
	})
	if d.draft.Invitation != nil && d.draft.Invitation.AvatarSkipped && kit.Button(c, nt("retryInvitation"), icons.RefreshCw, false).Clicked() {
		d.parent.mutate("profiles.update", map[string]any{"input": map[string]any{"id": d.botID, "retryInvitation": true}}, nil)
	}
}
func (d *profileSettings) advanced(c *ui.Context) {
	kit.Help(c, d.draft.HomeDir).Selectable()
	if kit.Button(c, bt("bots.homeFolder.open"), icons.FolderOpen, false).Disabled(d.draft.HomeDir == "").Clicked() {
		d.parent.mutate("profiles.openHome", map[string]any{"botId": d.botID}, nil)
	}
}
func (d *profileSettings) modelFields(c *ui.Context) {
	caps := &d.draft.Capabilities.Value
	chain := caps.ModelChain
	following := bytes.Equal(bytes.TrimSpace(caps.ModelChainOverride), []byte("null"))
	if following {
		chain = copyValue(d.parent.defaultChain)
		kit.Help(c, nt("followingDefault"))
	}
	d.chainFields(c, &chain, func() { d.setPrimary(chain); d.changed() })
	if kit.Button(c, bt("bots.model.restoreDefault"), icons.RotateCcw, false).Clicked() {
		caps.ModelChainOverride = json.RawMessage("null")
		caps.ModelOverride = json.RawMessage("null")
		caps.ModelChain = copyValue(d.parent.defaultChain)
		if len(caps.ModelChain) > 0 {
			d.mirrorPrimary(caps.ModelChain[0])
		}
		d.changed()
	}
	kit.Heading(c, bt("bots.model.task"))
	inherit := caps.TaskModelOverride == nil
	hint := ""
	if inherit {
		hint = bt("bots.model.inheritingPrimary")
	}
	if kit.Toggle(c, &inherit, bt("bots.model.inheritPrimary"), hint).Changed() {
		if inherit {
			caps.TaskModelOverride = nil
		} else {
			route := ModelRoute{Harness: "pi", FastMode: json.RawMessage("false")}
			if len(chain) > 0 {
				route = copyValue(chain[0])
			}
			caps.TaskModelOverride = &route
		}
		d.changed()
	}
	if caps.TaskModelOverride != nil {
		d.routeFields(c, caps.TaskModelOverride, func() { d.changed() })
	}
}
func (d *profileSettings) setPrimary(chain []ModelRoute) {
	caps := &d.draft.Capabilities.Value
	caps.ModelChain = copyValue(chain)
	raw, _ := json.Marshal(chain)
	caps.ModelChainOverride = raw
	if len(chain) > 0 {
		d.mirrorPrimary(chain[0])
		raw, _ := json.Marshal(chain[0])
		caps.ModelOverride = raw
	}
}
func (d *profileSettings) mirrorPrimary(route ModelRoute) {
	caps := &d.draft.Capabilities.Value
	caps.Model, caps.ProviderID, caps.Effort, caps.FastMode, caps.Harness = route.Model, route.ProviderID, route.Effort, route.FastMode, "pi"
}
func (d *profileSettings) routeFields(c *ui.Context, route *ModelRoute, changed func()) {
	provider := ""
	if route.ProviderID != nil {
		provider = *route.ProviderID
	}
	value := widgets.AISelection{Provider: provider, Model: route.Model}
	selected, picked := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: d.parent.env, Groups: widgets.ProviderGroups(d.parent.providers), Selected: widgets.SelectionValue(value), Label: nt("model")}, Placeholder: nt("chooseModel")})
	if picked {
		selection, ok := widgets.ParseSelectionValue(selected)
		if ok {
			route.ProviderID = &selection.Provider
			route.Model = selection.Model
			route.Harness = "pi"
			changed()
		}
	}
	efforts := []string{}
	for _, p := range d.parent.providers {
		if p.ID == provider {
			metadata := p.ModelMetadata[route.Model]
			efforts = append(efforts, metadata.Efforts...)
			efforts = append(efforts, metadata.Levels...)
		}
	}
	options := []kit.SelectOption{{Value: "", Label: nt("defaultEffort")}}
	seen := map[string]bool{"": true}
	for _, effort := range append(efforts, route.Effort) {
		if !seen[effort] {
			seen[effort] = true
			options = append(options, kit.SelectOption{Value: effort, Label: effort})
		}
	}
	ui.Field(c, nt("effort"), func() {
		if picked, _ := kit.Select(c, &route.Effort, options); picked {
			changed()
		}
	})
	fast := bytes.Equal(bytes.TrimSpace(route.FastMode), []byte("true"))
	if kit.Toggle(c, &fast, nt("fastMode"), "").Changed() {
		raw, _ := json.Marshal(fast)
		route.FastMode = raw
		changed()
	}
}
func (d *profileSettings) chainFields(c *ui.Context, chain *[]ModelRoute, changed func()) {
	for index := range *chain {
		if index >= len(*chain) {
			continue
		}
		i := index
		ui.Column(c.Key(i)).FillWidth().Gap(8).Margin(0, 0, 12).Children(func() {
			ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
				kit.Strong(c, bt("bots.modelChain.routeLabel", "index", i+1)).Grow(1)
				if kit.IconButton(c, icons.ArrowUp, bt("bots.modelChain.moveUp")).Disabled(i == 0).Clicked() {
					(*chain)[i], (*chain)[i-1] = (*chain)[i-1], (*chain)[i]
					changed()
				}
				if kit.IconButton(c, icons.ArrowDown, nt("moveDown")).Disabled(i == len(*chain)-1).Clicked() {
					(*chain)[i], (*chain)[i+1] = (*chain)[i+1], (*chain)[i]
					changed()
				}
				if kit.IconButton(c, icons.X, bt("bots.modelChain.remove")).Disabled(len(*chain) <= 1).Clicked() {
					*chain = slices.Delete(*chain, i, i+1)
					changed()
				}
			})
			if i < len(*chain) {
				d.routeFields(c, &(*chain)[i], changed)
			}
		})
	}
	if kit.Button(c, bt("bots.modelChain.add"), icons.Plus, false).Disabled(len(*chain) >= 5).Clicked() {
		route := ModelRoute{Harness: "pi", FastMode: json.RawMessage("false")}
		if len(*chain) > 0 {
			route = copyValue((*chain)[0])
		}
		*chain = append(*chain, route)
		changed()
	}
}
func (d *profileSettings) effectiveChain() []ModelRoute {
	caps := d.draft.Capabilities.Value
	if bytes.Equal(bytes.TrimSpace(caps.ModelChainOverride), []byte("null")) {
		return copyValue(d.parent.defaultChain)
	}
	return copyValue(caps.ModelChain)
}
func (d *profileSettings) restoreDefaultModels() {
	captured := copyValue(d.chain)
	d.saving = true
	oneBotCall(d.parent, "modelChain.reset", nil, func(value struct {
		ModelChain   []ModelRoute `json:"modelChain"`
		IsCustomized bool         `json:"isCustomized"`
	}, err error) {
		if d.closed {
			return
		}
		d.saving = false
		if err != nil {
			d.error = err.Error()
			return
		}
		// Route fields stay editable while reset is pending. Only the captured
		// buffer can be replaced; newer typing remains an unsaved delta.
		d.chainBaseline = copyValue(value.ModelChain)
		if sameJSON(d.chain, captured) {
			d.chain = copyValue(value.ModelChain)
		}
		d.parent.defaultChain, d.parent.chainCustomized = value.ModelChain, value.IsCustomized
		d.error = ""
		d.finishSave()
	})
}

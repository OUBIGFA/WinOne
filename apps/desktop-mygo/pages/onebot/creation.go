package onebot

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type creationState struct {
	prompt, name, description, identity, userContext, avatar, avatarImage, token, templateID string
	generationRoute                                                                          ModelRoute
	skills                                                                                   []string
	busy, generateAvatar                                                                     bool
	error                                                                                    string
	portrait                                                                                 *portraitDialog
}

func (d *creationState) Dirty() bool {
	return d.prompt != "" || d.name != "" || d.description != "" || d.identity != "" || d.avatarImage != "" || d.token != ""
}
func (p *page) beginCreate() {
	p.env.RequestChange(func() {
		if p.creation != nil {
			return
		}
		d := &creationState{avatar: "🐼", generationRoute: ModelRoute{Harness: "pi", FastMode: json.RawMessage("false")}}
		if len(p.defaultChain) > 0 {
			d.generationRoute = copyValue(p.defaultChain[0])
		}
		p.creation = d
	})
}
func (p *page) generateCreation(d *creationState) {
	if d.busy || strings.TrimSpace(d.prompt) == "" {
		return
	}
	if d.generationRoute.Model == "" {
		d.error = nt("modelRequired")
		return
	}
	d.busy = true
	request := map[string]any{"prompt": d.prompt, "name": d.name, "description": d.description, "modelRoute": map[string]any{"agentKind": "pi", "providerId": d.generationRoute.ProviderID, "model": d.generationRoute.Model}}
	if d.token != "" {
		request["token"] = d.token
	}
	oneBotCall(p, "bots.generateDraft", map[string]any{"request": request}, func(result struct {
		Token, Name, Description string
		Skills                   []string
	}, err error) {
		if p.creation != d {
			return
		}
		d.busy = false
		if err != nil {
			d.error = err.Error()
			return
		}
		d.token, d.name, d.description, d.skills = result.Token, result.Name, result.Description, result.Skills
		d.templateID = ""
		d.error = ""
	})
}
func (p *page) createCompanion(d *creationState) {
	if d.busy {
		return
	}
	if strings.TrimSpace(d.name) == "" {
		d.error = nt("nameRequired")
		return
	}
	for _, bot := range p.profiles {
		if bot.Status != "archived" && normalizedName(bot.Name) == normalizedName(d.name) {
			d.error = nt("nameTaken")
			return
		}
	}
	d.busy = true
	input := map[string]any{"name": d.name, "description": d.description, "avatar": d.avatar, "identitySource": d.identity, "userContextSource": d.userContext, "skills": d.skills, "locale": botLocale()}
	if d.templateID != "" {
		input["templateId"] = d.templateID
	}
	if d.avatarImage != "" {
		input["avatarImageBase64"] = d.avatarImage
	}
	if d.token != "" {
		input["creationDraftToken"] = d.token
		input["prepareInvitation"] = true
		input["generateAvatar"] = d.generateAvatar
	}
	oneBotCall(p, "profiles.create", map[string]any{"input": input}, func(bot Profile, err error) {
		if p.creation != d {
			return
		}
		d.busy = false
		if err != nil {
			d.error = err.Error()
			return
		}
		p.creation = nil
		p.mergeProfile(bot)
		p.refresh()
		if canonical(bot) != "" {
			p.showConversation(bot.ID, canonical(bot))
		} else {
			p.openSettings(bot.ID, "home")
		}
	})
}
func (p *page) createView(c *ui.Context) {
	d := p.creation
	if d == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("create"), Width: 720, Confirm: nt("createConfirm"), Cancel: nt("cancel"), Busy: d.busy, Error: d.error, ConfirmDisabled: strings.TrimSpace(d.name) == "", Body: func() {
		ui.Field(c, nt("describeCompanion"), func() {
			ui.TextArea(c, &d.prompt).FillWidth().Height(100).Label(nt("describeCompanion")).Disabled(d.busy)
		})
		selection := widgets.AISelection{Model: d.generationRoute.Model}
		if d.generationRoute.ProviderID != nil {
			selection.Provider = *d.generationRoute.ProviderID
		}
		if value, changed := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: p.env, Groups: widgets.ProviderGroups(p.providers), Selected: widgets.SelectionValue(selection), Label: nt("generationModel")}, Disabled: d.busy, Placeholder: nt("chooseModel")}); changed {
			if chosen, ok := widgets.ParseSelectionValue(value); ok {
				d.generationRoute.ProviderID = &chosen.Provider
				d.generationRoute.Model = chosen.Model
			}
		}
		if kit.Button(c, nt("generateDraft"), icons.Sparkles, d.busy).Disabled(d.busy || strings.TrimSpace(d.prompt) == "").Clicked() {
			p.generateCreation(d)
		}
		ui.Field(c, nt("name"), func() { ui.TextInput(c, &d.name).FillWidth().Label(nt("name")).Disabled(d.busy) })
		ui.Field(c, nt("description"), func() {
			ui.TextArea(c, &d.description).FillWidth().Height(100).Label(nt("description")).Disabled(d.busy)
		})
		ui.Field(c, bt("bots.profile.personality"), func() {
			ui.TextArea(c, &d.identity).FillWidth().Height(120).Label(bt("bots.profile.personality")).Disabled(d.busy)
		})
		ui.Field(c, bt("bots.memory.aboutMe"), func() {
			ui.TextArea(c, &d.userContext).FillWidth().Height(100).Label(bt("bots.memory.aboutMe")).Disabled(d.busy)
		})
		ui.Field(c, nt("avatarEmoji"), func() { ui.TextInput(c, &d.avatar).FillWidth().Label(nt("avatarEmoji")).Disabled(d.busy) })
		button := kit.Button(c, bt("bots.profile.changeAvatar"), icons.Camera, false).Disabled(d.busy)
		pick := func(path string) { p.creationPortrait(d, path) }
		if button.Clicked() {
			core.Go(p.env, func() (string, error) {
				return p.env.Dialogs.PickFile(bt("bots.profile.changeAvatar"), core.Filter{Name: nt("images"), Extensions: []string{"png", "jpg", "jpeg", "webp"}})
			}, func(path string, err error) {
				if p.creation != d {
					return
				}
				if err != nil {
					d.error = err.Error()
				} else if path != "" {
					pick(path)
				}
			})
		}
		widgets.DropArea(c, p.env, button, widgets.DropFiles, false, d.busy, func(paths []string, err error) {
			if err != nil {
				d.error = err.Error()
			} else if len(paths) > 0 {
				pick(paths[0])
			}
		})
		if d.avatarImage != "" {
			kit.Help(c, nt("avatarSelected"))
		}
		if d.token != "" {
			kit.Toggle(c, &d.generateAvatar, nt("generatePortrait"), nt("generatePortraitHint"))
			if kit.Button(c, nt("generatePortraitNow"), icons.Sparkles, d.busy).Disabled(d.busy).Clicked() {
				d.busy = true
				oneBotCall(p, "bots.generateAvatar", map[string]string{"token": d.token}, func(value struct {
					AvatarImageBase64 string `json:"avatarImageBase64"`
				}, err error) {
					if p.creation != d {
						return
					}
					d.busy = false
					if err != nil {
						d.error = err.Error()
						return
					}
					d.avatarImage = value.AvatarImageBase64
				})
			}
		}
		for _, skill := range d.skills {
			kit.Help(c, skill)
		}
	}}) {
		p.createCompanion(d)
	}
	if !open && !d.busy {
		p.env.RequestChange(func() {
			if d.portrait != nil {
				d.portrait.Close()
			}
			p.creation = nil
		})
	}
	if d.portrait != nil {
		d.portrait.View(c)
	}
}
func (p *page) creationPortrait(d *creationState, path string) {
	oneBotCall(p, "avatar.source", map[string]string{"path": path}, func(value struct {
		Base64 string `json:"base64"`
	}, err error) {
		if p.creation != d {
			return
		}
		if err != nil {
			d.error = err.Error()
			return
		}
		portrait, err := newPortraitDialog(p, value.Base64, func(image string, done func(error)) { d.avatarImage = image; done(nil) }, func() { d.portrait = nil })
		if err != nil {
			d.error = err.Error()
			return
		}
		d.portrait = portrait
	})
}

package automations

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (p *Page) showTemplates() {
	p.templateOpen = true
	p.templateErr = ""
	if p.busy["templates"] {
		return
	}
	p.busy["templates"] = true
	type response struct {
		Templates []Template `json:"templates"`
	}
	core.Go(p.env, func() (response, error) {
		var r response
		err := p.call("templates", map[string]string{"language": locale()}, &r)
		return r, err
	}, func(r response, err error) {
		delete(p.busy, "templates")
		if p.closed {
			return
		}
		if err != nil {
			p.templateErr = err.Error()
			return
		}
		p.templates = r.Templates
	})
}
func (p *Page) templatesView(c *ui.Context) {
	if !p.templateOpen {
		return
	}
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: tr("templates"), Cancel: tr("close"), Error: p.templateErr, Width: 760, Busy: p.busy["template.apply"], Body: func() {
		kit.TextInput(c, &p.templateSearch).Label(tr("search")).Placeholder(tr("search")).FillWidth()
		for _, t := range p.templates {
			if p.templateSearch != "" && !strings.Contains(strings.ToLower(t.Name+" "+t.Description+" "+t.Category), strings.ToLower(p.templateSearch)) {
				continue
			}
			ui.Column(c.Key(t.ID)).FillWidth().Gap(8).Padding(8).Children(func() {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					if kit.Button(c, t.Name, icons.LayoutTemplate, false).Grow(1).Clicked() {
						p.templateID = t.ID
						p.templateParams = map[string]string{}
						for _, param := range t.Parameters {
							p.templateParams[param.Key] = param.Default
						}
					}
					if t.Source == "user" && kit.IconButton(c, icons.Trash2, tr("deleteTemplate")).Clicked() {
						p.confirmation = &confirmation{title: tr("deleteTemplate"), message: t.Name, action: "template.delete", id: t.ID}
					}
				})
				kit.Help(c, t.Description)
				if p.templateID == t.ID {
					if t.Prompt != "" {
						ui.Text(c, t.Prompt).Selectable().FillWidth()
					}
					for _, param := range t.Parameters {
						value := p.templateParams[param.Key]
						ui.Column(c.Key(param.Key)).FillWidth().Gap(4).Children(func() {
							switch param.Type {
							case "select":
								options := []kit.SelectOption{}
								for _, v := range param.Options {
									options = append(options, kit.SelectOption{Value: v, Label: v})
								}
								choice(c, param.Label, &value, options...)
							case "boolean":
								enabled := value == "true"
								if kit.Toggle(c, &enabled, param.Label, "").Changed() {
									if enabled {
										value = "true"
									} else {
										value = "false"
									}
								}
							default:
								field(c, param.Label, &value, false)
							}
						})
						p.templateParams[param.Key] = value
					}
					if kit.PrimaryButton(c, tr("useTemplate"), icons.Plus, p.busy["template.apply"]).Clicked() {
						p.applyTemplate(t)
					}
				}
			})
		}
	}})
	if !open {
		p.templateOpen = false
	}
}
func (p *Page) applyTemplate(t Template) {
	if p.busy["template.apply"] {
		return
	}
	if p.Dirty() {
		p.templateErr = tr("draftExists")
		return
	}
	values := map[string]string{}
	for key, value := range p.templateParams {
		values[key] = value
	}
	for _, param := range t.Parameters {
		if param.Required && values[param.Key] == "" && param.Default == "" {
			p.templateErr = tr("parameterRequired", "name", param.Label)
			return
		}
	}
	// Capture the editor identity and content, since the asynchronous apply
	// must not replace a form opened or edited while the Host was replying.
	editor := p.draft
	var original Definition
	var buffers string
	if editor != nil {
		raw, _ := json.Marshal(editor.Value)
		_ = json.Unmarshal(raw, &original)
		buffers = editor.buffers()
	}
	p.busy["template.apply"] = true
	core.Go(p.env, func() (json.RawMessage, error) {
		return p.env.Host.Call("chat.automations.template.apply", map[string]any{"id": t.ID, "language": locale(), "parameters": values})
	}, func(raw json.RawMessage, err error) {
		delete(p.busy, "template.apply")
		if p.closed {
			return
		}
		if err != nil {
			p.templateErr = err.Error()
			return
		}
		if p.draft != editor || editor != nil && (!reflect.DeepEqual(editor.Value, original) || editor.buffers() != buffers) {
			p.templateErr = tr("draftExists")
			return
		}
		d := newDraft(nil, p.runtime.Timezone)
		if err = json.Unmarshal(raw, &d.Value); err != nil {
			p.templateErr = err.Error()
			return
		}
		// Reuse the editor's hydration for all template modes/timeouts, while
		// retaining the pristine-new baseline so the applied draft is protected.
		hydrated := newDraft(&Schedule{Definition: d.Value}, p.runtime.Timezone)
		hydrated.original, hydrated.originalBuffers = d.original, d.originalBuffers
		p.draft = hydrated
		p.templateOpen = false
		p.loadFormResources()
	})
}
func (p *Page) saveTemplate() {
	d := p.draft
	if d == nil || p.busy["template.save:"] {
		return
	}
	if err := d.validate(); err != nil {
		d.err = err.Error()
		return
	}
	value := objectOf(d.Value)
	value["description"] = d.Value.Name
	value["category"] = "custom"
	p.mutate("template.save", "", map[string]any{"template": value}, func() {
		if p.env.Toast != nil {
			p.env.Toast(tr("templateSaved"))
		}
	})
}
func (p *Page) importsView(c *ui.Context) {
	if !p.importOpen {
		return
	}
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: tr("projectImports"), Message: tr("importHint"), Cancel: tr("close"), Width: 760, Body: func() {
		if len(p.imports) == 0 {
			kit.Help(c, tr("noImports"))
		}
		for _, item := range p.imports {
			ui.Column(c.Key(item.WorkingDir)).FillWidth().Gap(10).Children(func() {
				kit.Label(c, item.WorkingDir).Selectable()
				for _, raw := range item.Schedules {
					var schedule struct {
						Name     string `json:"name"`
						Prompt   string `json:"prompt"`
						CronExpr string `json:"cronExpr"`
					}
					if err := json.Unmarshal(raw, &schedule); err != nil {
						kit.InlineError(c, err.Error())
						continue
					}
					ui.Column(c).Gap(4).Children(func() {
						kit.Label(c, schedule.Name)
						kit.Help(c, schedule.CronExpr)
						ui.Text(c, schedule.Prompt).Selectable().FillWidth()
					})
				}
				for _, name := range item.RemovedNames {
					kit.Help(c, tr("willRemove", "name", name))
				}
				if kit.PrimaryButton(c, tr("approveImport"), icons.Check, p.busy["project.reconcile:"+item.WorkingDir]).Clicked() {
					p.mutate("project.reconcile", item.WorkingDir, map[string]string{"workingDir": item.WorkingDir, "approvedHash": item.ConfigHash}, nil)
				}
			})
		}
	}})
	if !open {
		p.importOpen = false
	}
}

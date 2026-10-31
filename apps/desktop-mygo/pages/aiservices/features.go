package aiservices

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// pluginOptions is a bundled plugin's options as typed: JSON that applies
// once it is an object; until then Save waits and the text stays.
type pluginOptions struct {
	text, of string
	invalid  bool
}

// features are Magpie's built-in plugins and the bundled ones, adapted
// from Magpie 674b6ca renderBuiltins/ui.form: every manifest and schema
// field, edited in the page's draft rather than per-account files.
func (a *advancedPanel) features(c *ui.Context, catalog *magpieCatalog, locked bool) {
	p := a.p
	kit.Help(c, i18n.T("aiPlugins.hint"))
	if a.catalog == nil {
		if a.loading {
			kit.Status(c, i18n.T("aiPlugins.loading"))
		}
		return
	}
	ui.Column(c).Disabled(p.navigationLocked()).Children(func() {
		kit.SubTabs(c, &a.builtinTab, []string{i18n.T("aiPlugins.builtin"), i18n.T("aiPlugins.bundled")}, p.env.RequestChange)
	})
	if a.builtinTab == 0 {
		a.builtins(c, catalog, locked)
	} else {
		a.bundled(c, catalog, locked)
	}
	invalid := false
	for _, o := range a.options {
		invalid = invalid || o.invalid
	}
	p.pluginInvalid = invalid
}

func (a *advancedPanel) builtins(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	preset := s.BuiltinPreset
	if preset == "" {
		preset = "all"
	}
	kit.Group(c, icons.Blocks, i18n.T("aiPlugins.preset"), i18n.T("aiPlugins.presetHint"), func() {
		if choiceRow(c, i18n.T("aiPlugins.preset"), "", &preset, options("all", i18n.T("aiPlugins.all"), "essentials", i18n.T("aiPlugins.essentials")), locked) {
			s.BuiltinPreset = preset
		}
		kit.Stack(c, "", "", func() {
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("aiPlugins.overrides"), icons.RotateCcw, false).Disabled(locked || len(s.BuiltinPlugins) == 0).Clicked() {
					s.BuiltinPlugins = map[string]core.Preserved[builtinChoice]{}
				}
			})
		})
	})
	if len(catalog.BuiltinPlugins) == 0 {
		kit.Notice(c, kit.Info, "", i18n.T("aiPlugins.noPlugins"), nil)
	}
	for _, plugin := range catalog.BuiltinPlugins {
		plugin := plugin
		choice := s.BuiltinPlugins[plugin.ID].Value
		on := plugin.InUse > 0
		if !on {
			if choice.Enabled != nil {
				on = *choice.Enabled
			} else {
				on = preset == "all" || plugin.Essential
			}
		}
		key := "aiPlugins." + plugin.ID
		if plugin.ID == "tools" {
			key = "aiPlugins.pluginTools"
		}
		name := plugin.Name
		if i18n.Has(key) {
			name = i18n.T(key)
		}
		change := func(edit func(*builtinChoice)) {
			next := map[string]core.Preserved[builtinChoice]{}
			for k, v := range s.BuiltinPlugins {
				next[k] = v
			}
			d := next[plugin.ID]
			edit(&d.Value)
			next[plugin.ID] = d
			s.BuiltinPlugins = next
		}
		kit.Group(c.Key(plugin.ID), icons.Puzzle, name, plugin.Sub, func() {
			hint := plugin.Why
			if plugin.InUse > 0 {
				hint = i18n.T("aiPlugins.inUse")
			}
			enabled := on
			if kit.Toggle(c, &enabled, name, hint).Disabled(locked || plugin.InUse > 0).Changed() {
				change(func(d *builtinChoice) { d.Enabled = ptr(enabled) })
			}
			if on && plugin.Schema != nil {
				for _, field := range plugin.Schema.Order {
					field := field
					property, ok := plugin.Schema.Properties[field]
					if !ok {
						continue
					}
					current, set := choice.Config[field]
					if !set {
						current, set = plugin.Config[field]
					}
					if !set {
						current = property.Default
					}
					a.schemaField(c, plugin.ID+":"+field, property, current, locked, func(v any) {
						change(func(d *builtinChoice) {
							next := map[string]any{}
							for k, x := range d.Config {
								next[k] = x
							}
							next[field] = v
							d.Config = next
						})
					})
				}
			}
			if _, ok := a.opens["details:"+plugin.ID]; !ok {
				a.opens["details:"+plugin.ID] = false
			}
			open := a.opens["details:"+plugin.ID]
			kit.Stack(c, "", "", func() {
				ui.Collapsible(c, i18n.T("aiPlugins.details"), &open, func() {
					state := plugin.State
					if i18n.Has("aiPlugins." + state) {
						state = i18n.T("aiPlugins." + state)
					}
					kit.Help(c, plugin.ID+" · "+state)
					requires := strings.Join(plugin.Requires, ", ")
					if requires == "" {
						requires = "—"
					}
					kit.Help(c, i18n.T("aiPlugins.dependencies")+"："+requires)
				}).FontSize(kit.HelpSize(c))
			})
			a.opens["details:"+plugin.ID] = open
		})
	}
}

// schemaField edits one field of a plugin's settings as its schema types
// it: a switch, a choice, or text or a number.
func (a *advancedPanel) schemaField(c *ui.Context, key string, property schemaProperty, current any, locked bool, set func(any)) {
	switch {
	case property.Type == "boolean":
		on := current == true
		if kit.Toggle(c.Key(key), &on, property.Title, property.Description).Disabled(locked).Changed() {
			set(on)
		}
	case len(property.Enum) > 0:
		var items []kit.SelectOption
		for _, option := range property.Enum {
			text := fmt.Sprint(option)
			items = append(items, kit.SelectOption{Value: text, Label: text})
		}
		value := fmt.Sprint(current)
		kit.Row(c.Key(key), property.Title, property.Description, func() {
			changed, trigger := kit.Select(c, &value, items)
			trigger.Label(property.Title).Disabled(locked)
			if changed {
				if property.Type == "string" {
					set(value)
				} else if v, err := strconv.ParseFloat(value, 64); err == nil {
					set(v)
				}
			}
		})
	case property.Type == "string":
		field := a.fields.get("schema:" + key)
		text := ""
		if current != nil {
			text = fmt.Sprint(current)
		}
		if typed, edited := field.sync(text); edited {
			set(typed)
		}
		kit.Stack(c.Key(key), property.Title, property.Description, func() {
			ui.TextInput(c, &field.text).FillWidth().Label(property.Title).Disabled(locked)
		})
	default:
		low, high := -1e15, 1e15
		if property.Minimum != nil {
			low = *property.Minimum
		}
		if property.Maximum != nil {
			high = *property.Maximum
		}
		var shown *float64
		if v, ok := current.(float64); ok {
			shown = &v
		}
		kit.Row(c.Key(key), property.Title, property.Description, func() {
			ui.Column(c).Disabled(locked).Children(func() {
				if v, changed := numberText(c, shown, low, high, property.Type == "integer", "", property.Title); changed && v != nil {
					set(*v)
				}
			})
		})
	}
}

func (a *advancedPanel) bundled(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	lang := language()
	for _, plugin := range catalog.BundledPlugins {
		plugin := plugin
		choice, chosen := s.BundledPlugins[plugin.Directory]
		on := len(plugin.Providers) > 0
		if chosen {
			on = choice.Value.Enabled
		}
		change := func(edit func(*bundledChoice)) {
			next := map[string]core.Preserved[bundledChoice]{}
			for k, v := range s.BundledPlugins {
				next[k] = v
			}
			d, ok := next[plugin.Directory]
			if !ok {
				d.Value.Enabled = on
			}
			edit(&d.Value)
			next[plugin.Directory] = d
			s.BundledPlugins = next
		}
		kit.Group(c.Key(plugin.Directory), icons.Package, plugin.Name+" · "+plugin.Version, plugin.Summary[lang], func() {
			enabled := on
			if kit.Toggle(c, &enabled, plugin.Name, "").Disabled(locked).Changed() {
				change(func(d *bundledChoice) { d.Enabled = enabled })
			}
			kind := plugin.Kind
			if kind == "" {
				kind = "providers"
			}
			line := i18n.T("aiPlugins." + kind)
			if len(plugin.Providers) > 0 {
				line += " · " + strings.Join(plugin.Providers, ", ")
			}
			if _, ok := a.opens["bundled:"+plugin.Directory]; !ok {
				a.opens["bundled:"+plugin.Directory] = false
			}
			open := a.opens["bundled:"+plugin.Directory]
			kit.Stack(c, "", line, func() {
				ui.Collapsible(c, i18n.T("aiPlugins.details"), &open, func() {
					ui.Column(c).FillWidth().Gap(8).Padding(8, 0).Children(func() {
						kit.Help(c, plugin.Package).Selectable()
						a.optionsField(c, plugin.Directory, choice.Value.Options, locked, func(options map[string]any) {
							change(func(d *bundledChoice) { d.Options = options })
						})
					})
				}).FontSize(kit.HelpSize(c))
			})
			a.opens["bundled:"+plugin.Directory] = open
		})
	}
}

// optionsField edits a bundled plugin's options as JSON.
func (a *advancedPanel) optionsField(c *ui.Context, key string, options map[string]any, locked bool, set func(map[string]any)) {
	o := a.options[key]
	if o == nil {
		o = &pluginOptions{}
		a.options[key] = o
	}
	if options == nil {
		options = map[string]any{}
	}
	formatted, _ := json.MarshalIndent(options, "", "  ")
	if string(formatted) != o.of {
		// The saved value changed (a save, an undo): show it.
		o.of, o.text, o.invalid = string(formatted), string(formatted), false
	}
	ui.Field(c, i18n.T("aiPlugins.options"), func() {
		area := ui.TextArea(c, &o.text).FillWidth().MinHeight(110).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).
			Label(i18n.T("aiPlugins.options")).Disabled(locked)
		if area.Changed() {
			var parsed any
			object, ok := map[string]any(nil), false
			if json.Unmarshal([]byte(o.text), &parsed) == nil {
				object, ok = parsed.(map[string]any)
			}
			o.invalid = !ok
			if ok {
				set(object)
				// The new value is this text's; keep the user's spacing.
				formatted, _ := json.MarshalIndent(object, "", "  ")
				o.of = string(formatted)
			}
		}
		if o.invalid {
			kit.InlineError(c, i18n.T("aiPlugins.invalidOptions"))
		}
	})
}


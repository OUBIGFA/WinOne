package studybook

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type toolAction struct {
	ID, Group        string
	Schema           map[string]any
	Backend, Mutates bool
}
type toolCatalog struct {
	Actions     []toolAction
	Definitions map[string]any
}
type toolForm struct {
	errors                 map[string]string
	catalog                *toolCatalog
	selected, query, group string
	value                  map[string]any
	inputs                 map[string]string
	variants               map[string]string
	result                 any
	confirmed              bool
	dirty                  bool
}

func (b *Book) openTool(method string, params map[string]any) {
	apply := func() {
		b.tab = 4
		b.toolSection = 4
		b.form.selected = method
		b.form.query = ""
		// Normalize Go slices to the same JSON types used by the form renderer.
		raw, _ := json.Marshal(params)
		b.form.value = map[string]any{}
		_ = json.Unmarshal(raw, &b.form.value)
		b.form.inputs = map[string]string{}
		b.form.variants = map[string]string{}
		b.form.result = nil
		b.form.dirty = false
		b.form.confirmed = false
		b.form.errors = map[string]string{}
	}
	if b.form.catalog != nil {
		apply()
		return
	}
	b.act("winone.tools", map[string]any{}, func(raw json.RawMessage) {
		var catalog toolCatalog
		if err := json.Unmarshal(raw, &catalog); err != nil {
			b.err = err.Error()
			return
		}
		b.form.catalog = &catalog
		apply()
	})
}

func human(name string) string {
	translated := i18n.T("study.field." + name)
	if !strings.HasPrefix(translated, "study.field.") {
		return translated
	}
	var out []rune
	for i, r := range name {
		if r == '_' || r == '.' {
			out = append(out, ' ')
			continue
		}
		if i > 0 && unicode.IsUpper(r) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	if len(out) > 0 {
		out[0] = unicode.ToUpper(out[0])
	}
	return string(out)
}
func resolveSchema(schema map[string]any, definitions map[string]any) map[string]any {
	if ref, ok := schema["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
		var value any = definitions
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, ok := value.(map[string]any)
			if !ok {
				return schema
			}
			value = m[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
		}
		if resolved, ok := value.(map[string]any); ok {
			return resolved
		}
	}
	return schema
}
func defaultValue(schema map[string]any, definitions map[string]any) any {
	schema = resolveSchema(schema, definitions)
	for _, key := range []string{"oneOf", "anyOf"} {
		if alternatives, ok := schema[key].([]any); ok && len(alternatives) > 0 {
			first, _ := alternatives[0].(map[string]any)
			return defaultValue(first, definitions)
		}
	}
	if value, ok := schema["default"]; ok {
		return value
	}
	if value, ok := schema["const"]; ok {
		return value
	}
	if options, ok := schema["enum"].([]any); ok && len(options) > 0 {
		return options[0]
	}
	switch schema["type"] {
	case "object":
		return map[string]any{}
	case "array":
		return []any{}
	case "boolean":
		return false
	case "number", "integer":
		return float64(0)
	}
	return ""
}
func (f *toolForm) field(c *ui.Context, label, path string, schema map[string]any, value any, set func(any)) {
	schema = resolveSchema(schema, f.catalog.Definitions)
	if alternatives, ok := schema["anyOf"].([]any); ok {
		var actual []any
		for _, raw := range alternatives {
			candidate, _ := raw.(map[string]any)
			if candidate["type"] != "null" {
				actual = append(actual, raw)
			}
		}
		if len(actual) == 1 {
			candidate, _ := actual[0].(map[string]any)
			f.field(c, label, path, candidate, value, set)
			return
		}
		if len(actual) > 1 {
			schema = map[string]any{"oneOf": actual}
		}
	}
	if alternatives, ok := schema["oneOf"].([]any); ok && len(alternatives) > 0 {
		selected := f.variants[path]
		if selected == "" {
			selected = "0"
			if current, ok := value.(map[string]any); ok {
				for i, raw := range alternatives {
					candidate, _ := raw.(map[string]any)
					candidate = resolveSchema(candidate, f.catalog.Definitions)
					properties, _ := candidate["properties"].(map[string]any)
					match := len(current) > 0
					for key, got := range current {
						property, exists := properties[key].(map[string]any)
						if !exists {
							match = false
							break
						}
						if want, constant := property["const"]; constant && want != got {
							match = false
							break
						}
					}
					if match {
						selected = strconv.Itoa(i)
						break
					}
				}
			}
		}
		options := make([]kit.SelectOption, len(alternatives))
		for i, raw := range alternatives {
			candidate, _ := raw.(map[string]any)
			candidate = resolveSchema(candidate, f.catalog.Definitions)
			name, _ := candidate["title"].(string)
			if name == "" {
				if props, ok := candidate["properties"].(map[string]any); ok {
					if operation, ok := props["op"].(map[string]any); ok {
						if value, ok := operation["const"].(string); ok {
							name = human(value)
						}
					}
					keys := make([]string, 0, len(props))
					for key := range props {
						keys = append(keys, human(key))
					}
					sort.Strings(keys)
					if name == "" {
						name = strings.Join(keys, " / ")
					}
				}
			}
			if name == "" {
				name = strconv.Itoa(i + 1)
			}
			options[i] = kit.SelectOption{Value: strconv.Itoa(i), Label: name}
		}
		ui.Field(c, label, func() {
			if changed, _ := kit.Select(c, &selected, options); changed {
				f.variants[path] = selected
				index, _ := strconv.Atoi(selected)
				candidate, _ := alternatives[index].(map[string]any)
				value = defaultValue(candidate, f.catalog.Definitions)
				set(value)
			}
		}).FillWidth()
		index, _ := strconv.Atoi(selected)
		candidate, _ := alternatives[index].(map[string]any)
		f.field(c, label, path+"/variant", candidate, value, set)
		return
	}
	if choices, ok := schema["enum"].([]any); ok {
		selected := fmt.Sprint(value)
		options := make([]kit.SelectOption, len(choices))
		for i, choice := range choices {
			options[i] = kit.SelectOption{Value: fmt.Sprint(choice), Label: human(fmt.Sprint(choice))}
		}
		ui.Field(c, label, func() {
			if changed, _ := kit.Select(c, &selected, options); changed {
				for _, choice := range choices {
					if fmt.Sprint(choice) == selected {
						set(choice)
						break
					}
				}
			}
		}).FillWidth()
		return
	}
	typeName, _ := schema["type"].(string)
	if constant, ok := schema["const"]; ok {
		kit.Help(c, label+": "+human(fmt.Sprint(constant)))
		return
	}
	if typeName == "" {
		if _, ok := schema["properties"]; ok {
			typeName = "object"
		}
	}
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			object = map[string]any{}
			set(object)
		}
		properties, _ := schema["properties"].(map[string]any)
		required := map[string]bool{}
		if keys, ok := schema["required"].([]any); ok {
			for _, key := range keys {
				required[fmt.Sprint(key)] = true
			}
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			if key != "session" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			key := key
			child, _ := properties[key].(map[string]any)
			current, present := object[key]
			ui.Column(c.Key(path + "/" + key)).FillWidth().Gap(8).Children(func() {
				if !required[key] {
					enabled := present
					if ui.Checkbox(c, &enabled, human(key)).Changed() {
						if enabled {
							object[key] = defaultValue(child, f.catalog.Definitions)
						} else {
							delete(object, key)
						}
						f.dirty = true
					}
					if !enabled {
						return
					}
					current = object[key]
				} else if !present {
					current = defaultValue(child, f.catalog.Definitions)
					object[key] = current
				}
				f.field(c, human(key), path+"/"+key, child, current, func(next any) { object[key] = next; f.dirty = true })
			})
		}
		if additional, ok := schema["additionalProperties"].(map[string]any); ok {
			keys := make([]string, 0, len(object))
			for key := range object {
				if _, known := properties[key]; !known {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				key := key
				f.field(c, human(key), path+"/"+key, additional, object[key], func(next any) { object[key] = next; f.dirty = true })
				if kit.GhostButton(c.Key("remove-"+key), text("remove"), icons.Trash2, false).Clicked() {
					delete(object, key)
					f.dirty = true
				}
			}
			key := f.inputs[path+"/newKey"]
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				if kit.TextInput(c, &key).Label(text("fieldName")).Grow(1).MinWidth(0).Changed() {
					f.inputs[path+"/newKey"] = key
				}
				if kit.Button(c, text("addField"), icons.Plus, false).Disabled(strings.TrimSpace(key) == "").Clicked() {
					object[key] = defaultValue(additional, f.catalog.Definitions)
					f.inputs[path+"/newKey"] = ""
					f.dirty = true
				}
			})
		}
	case "array":
		items, _ := value.([]any)
		child, _ := schema["items"].(map[string]any)
		kit.Label(c, label)
		for i, item := range items {
			i := i
			ui.Column(c.Key(path + "/" + strconv.Itoa(i))).FillWidth().Padding(12).Gap(8).Children(func() {
				f.field(c, label+" "+strconv.Itoa(i+1), path+"/"+strconv.Itoa(i), child, item, func(next any) { items[i] = next; set(items) })
				if kit.GhostButton(c, text("remove"), icons.Trash2, false).Clicked() {
					set(append(items[:i:i], items[i+1:]...))
				}
			})
		}
		if kit.Button(c, text("addItem"), icons.Plus, false).Clicked() {
			set(append(items, defaultValue(child, f.catalog.Definitions)))
		}
	case "boolean":
		checked, _ := value.(bool)
		if ui.Checkbox(c, &checked, label).Changed() {
			set(checked)
		}
	default:
		current, known := f.inputs[path]
		if !known {
			if value != nil {
				current = fmt.Sprint(value)
			}
		}
		ui.Field(c, label, func() {
			if ui.TextArea(c, &current).Lines(1, 8).Changed() {
				f.inputs[path] = current
				f.dirty = true
				if typeName == "number" || typeName == "integer" {
					number, err := strconv.ParseFloat(current, 64)
					if f.errors == nil {
						f.errors = map[string]string{}
					}
					if err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) && (typeName != "integer" || math.Trunc(number) == number) {
						delete(f.errors, path)
						set(number)
					} else {
						f.errors[path] = text("invalidNumber")
					}
				} else {
					set(current)
				}
			}
		}).FillWidth()
		if f.errors[path] != "" {
			kit.Help(c, f.errors[path])
		}
	}
}
func (b *Book) advancedTools(c *ui.Context) {
	if b.form.catalog == nil {
		if kit.Button(c, text("allTools"), icons.Settings, false).Clicked() {
			b.act("winone.tools", map[string]any{}, func(raw json.RawMessage) {
				var catalog toolCatalog
				if err := json.Unmarshal(raw, &catalog); err != nil {
					b.err = err.Error()
					return
				}
				b.form.catalog = &catalog
				b.form.value = map[string]any{}
				b.form.inputs = map[string]string{}
				b.form.variants = map[string]string{}
			})
		}
		return
	}
	f := &b.form
	groups := []kit.SelectOption{{Value: "", Label: text("allGroups")}}
	seen := map[string]bool{}
	for _, action := range f.catalog.Actions {
		if !seen[action.Group] {
			seen[action.Group] = true
			groups = append(groups, kit.SelectOption{Value: action.Group, Label: text("group." + action.Group)})
		}
	}
	kit.Select(c, &f.group, groups)
	kit.TextInput(c, &f.query).FillWidth().Placeholder(text("findTool")).Label(text("findTool"))
	var options []kit.SelectOption
	for _, action := range f.catalog.Actions {
		label := human(action.ID)
		if (f.group == "" || action.Group == f.group) && (f.query == "" || strings.Contains(strings.ToLower(label+action.ID), strings.ToLower(f.query))) {
			options = append(options, kit.SelectOption{Value: action.ID, Label: label})
		}
	}
	selected := f.selected
	if changed, _ := kit.Select(c, &selected, options); changed {
		change := func() {
			f.selected = selected
			f.value = map[string]any{}
			f.inputs = map[string]string{}
			f.variants = map[string]string{}
			f.result = nil
			f.dirty = false
			f.errors = map[string]string{}
		}
		if b.env.RequestChange != nil {
			b.env.RequestChange(change)
		} else {
			change()
		}
	}
	for _, action := range f.catalog.Actions {
		if action.ID != f.selected {
			continue
		}
		action := action
		f.field(c, human(action.ID), action.ID, action.Schema, f.value, func(value any) { f.value, _ = value.(map[string]any); f.dirty = true })
		destructive := strings.Contains(strings.ToLower(action.ID), "delete") || strings.Contains(strings.ToLower(action.ID), "remove") || strings.Contains(strings.ToLower(action.ID), "trash") || strings.Contains(strings.ToLower(action.ID), "reset")
		if destructive {
			ui.Checkbox(c, &f.confirmed, text("confirmAction"))
		}
		if kit.Button(c, text("execute"), icons.Play, false).Disabled(len(f.errors) > 0 || destructive && !f.confirmed).Clicked() {
			method, params := action.ID, any(f.value)
			if action.Backend {
				method = "winone.backend.call"
				params = map[string]any{"method": action.ID, "params": f.value}
			}
			b.act(method, params, func(raw json.RawMessage) {
				if err := json.Unmarshal(raw, &f.result); err != nil {
					b.err = err.Error()
					return
				}
				f.dirty = false
				f.confirmed = false
			})
		}
		break
	}
	if f.result != nil {
		kit.Heading(c, text("result"))
		renderValue(c, "", f.result)
	}
}
func renderValue(c *ui.Context, label string, value any) {
	switch item := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			renderValue(c.Key(key), human(key), item[key])
		}
	case []any:
		if label != "" {
			kit.Label(c, label)
		}
		for i, entry := range item {
			renderValue(c.Key(i), strconv.Itoa(i+1), entry)
		}
		if len(item) == 0 {
			kit.Help(c, text("none"))
		}
	default:
		shown := fmt.Sprint(value)
		if value == nil {
			shown = text("none")
		}
		if label != "" {
			shown = label + ": " + shown
		}
		ui.Text(c, shown).Selectable()
	}
}

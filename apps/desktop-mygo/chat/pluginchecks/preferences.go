package pluginchecks

import (
	"bytes"
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

func (m *Manager) loadModels() {
	if m.modelsLoading {
		return
	}
	m.modelsLoading = true
	core.Go(m.env, func() ([]widgets.AIProvider, error) {
		var value struct {
			Providers []widgets.AIProvider `json:"providers"`
		}
		err := m.env.Host.CallInto("capture.ai.configuration.get", nil, &value)
		return value.Providers, err
	}, func(value []widgets.AIProvider, err error) {
		if !m.open {
			return
		}
		m.modelsLoading = false
		if err != nil {
			m.error = err.Error()
			return
		}
		m.providers = value
	})
}

// Plugin defaults use the same model menu as Chat. Effort names come from the
// selected provider; unavailable saved values remain visible until changed.
func (m *Manager) taskFields(c *ui.Context) {
	value := &m.taskPreferences
	kit.Strong(c, text("taskDefaults"))
	kit.Help(c, text("taskDefaultsHint"))
	selected := ""
	if value.Model != "" {
		selected = widgets.SelectionValue(widgets.AISelection{Provider: value.ProviderID, Model: value.Model})
	}
	ui.Field(c, text("taskModel"), func() {
		next, changed := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{
			Env: m.env, Groups: widgets.ProviderGroups(m.providers), Selected: selected, Label: text("taskModel"), Choices: []widgets.ModelChoice{{Value: "", Label: text("taskDefaultModel")}}}, Disabled: m.busy || m.modelsLoading})
		if changed {
			if choice, ok := widgets.ParseSelectionValue(next); ok {
				value.AgentKind = "pi"
				value.ProviderID, value.Model = choice.Provider, choice.Model
			} else {
				value.AgentKind, value.ProviderID, value.Model = "", "", ""
			}
		}
	})
	if value.AgentKind != "" && value.AgentKind != "pi" {
		kit.InlineError(c, text("taskLegacyHarness"))
	}
	efforts := []kit.SelectOption{{Value: "", Label: text("taskDefaultEffort")}}
	seen := map[string]bool{"": true}
	add := func(word string) {
		if !seen[word] {
			seen[word] = true
			efforts = append(efforts, kit.SelectOption{Value: word, Label: word})
		}
	}
	for _, provider := range m.providers {
		if provider.ID == value.ProviderID {
			metadata := provider.ModelMetadata[value.Model]
			for _, word := range metadata.Efforts {
				add(word)
			}
			for _, word := range metadata.Levels {
				add(word)
			}
		}
	}
	add(value.Effort)
	ui.Field(c, text("taskEffort"), func() { kit.Select(c, &value.Effort, efforts) })
	fast := ""
	if len(value.FastMode) > 0 && !bytes.Equal(value.FastMode, []byte("null")) {
		if bytes.Equal(value.FastMode, []byte("true")) {
			fast = "true"
		} else if bytes.Equal(value.FastMode, []byte("false")) {
			fast = "false"
		} else {
			_ = json.Unmarshal(value.FastMode, &fast)
		}
	}
	fastOptions := []kit.SelectOption{{Value: "", Label: text("taskFollowDefault")}, {Value: "false", Label: text("taskFastOff")}, {Value: "true", Label: text("taskFastOn")}}
	if fast != "" && fast != "true" && fast != "false" {
		fastOptions = append(fastOptions, kit.SelectOption{Value: fast, Label: fast})
	}
	ui.Field(c, text("taskFastMode"), func() {
		if changed, _ := kit.Select(c, &fast, fastOptions); changed {
			switch fast {
			case "":
				value.FastMode = nil
			case "true":
				value.FastMode = json.RawMessage("true")
			case "false":
				value.FastMode = json.RawMessage("false")
			default:
				value.FastMode, _ = json.Marshal(fast)
			}
		}
	})
	permissions := []kit.SelectOption{{Value: "", Label: text("taskDefaultPermission")}, {Value: "ask", Label: text("taskAsk")}, {Value: "auto", Label: text("taskAuto")}, {Value: "bypassPermissions", Label: text("taskFull")}}
	if value.PermissionMode == "acceptEdits" {
		permissions = append(permissions, kit.SelectOption{Value: "acceptEdits", Label: text("taskAcceptEdits")})
	}
	if value.PermissionMode == "plan" {
		permissions = append(permissions, kit.SelectOption{Value: "plan", Label: text("taskLegacyPlan")})
	}
	ui.Field(c, text("taskPermission"), func() { kit.Select(c, &value.PermissionMode, permissions) })
	ui.Field(c, text("taskDirectory"), func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			field := kit.TextInput(c, &value.WorkingDir).Grow(1).MinWidth(0).Label(text("taskDirectory")).Placeholder(text("taskDirectoryHint"))
			widgets.DropArea(c, m.env, field, widgets.DropFolders, false, m.busy, func(paths []string, err error) {
				if err != nil {
					m.error = err.Error()
				} else if len(paths) > 0 && m.configuration != nil {
					m.taskPreferences.WorkingDir = paths[0]
				}
			})
			if kit.Button(c, text("taskBrowse"), icons.FolderOpen, false).Disabled(m.busy).Clicked() {
				id := m.configuration.PluginID
				core.Go(m.env, func() (string, error) { return m.env.Dialogs.PickFolder(text("taskDirectory")) }, func(path string, err error) {
					if !m.open || m.configuration == nil || m.configuration.PluginID != id {
						return
					}
					if err != nil {
						m.error = err.Error()
					} else if path != "" {
						m.taskPreferences.WorkingDir = path
					}
				})
			}
		})
	})
}

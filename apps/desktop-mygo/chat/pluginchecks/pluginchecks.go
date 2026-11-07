// Package pluginchecks is the shared native plugin picker/management UI for
// Cindy schedule and routine pre-run checks. The Host owns every mutation.
package pluginchecks

import (
	"encoding/json"
	"strings"
	"winone/desktopmygo/chat/pluginhost/globalpages"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type Check struct {
	PluginID  string `json:"pluginId"`
	OnFailure string `json:"onFailure"`
}

type Candidate struct {
	PluginID string `json:"pluginId"`
	Name     string `json:"name"`
}

type Picker struct {
	rows            []Candidate
	loaded, loading bool
	error           string
	manager         *Manager
}

func text(key string, args ...any) string { return i18n.T("pluginChecks."+key, args...) }

func (p *Picker) refresh(env *core.Env, point string) {
	if p.loading {
		return
	}
	p.loading = true
	core.Go(env, func() ([]Candidate, error) {
		var rows []Candidate
		err := env.Host.CallInto("chat.ghosts.candidates", map[string]string{"point": point}, &rows)
		return rows, err
	}, func(rows []Candidate, err error) {
		p.loading, p.loaded = false, true
		if err != nil {
			p.error = err.Error()
			return
		}
		p.rows, p.error = rows, ""
	})
}

// View preserves an unavailable saved plugin and its failure policy. It never
// silently chooses the first candidate or removes a legacy check.
func (p *Picker) View(c *ui.Context, env *core.Env, point string, value **Check) {
	if !p.loaded && !p.loading {
		p.refresh(env, point)
	}
	enabled := *value != nil
	if kit.Toggle(c, &enabled, text("enabled"), text("hint")).Changed() {
		if enabled {
			*value = &Check{OnFailure: "block"}
		} else {
			*value = nil
		}
	}
	if *value != nil {
		check := *value
		options := []kit.SelectOption{{Value: "", Label: text("none")}}
		found := check.PluginID == ""
		for _, row := range p.rows {
			options = append(options, kit.SelectOption{Value: row.PluginID, Label: row.Name})
			found = found || row.PluginID == check.PluginID
		}
		if !found {
			options = append(options, kit.SelectOption{Value: check.PluginID, Label: text("missing", "id", check.PluginID)})
		}
		ui.Field(c, text("plugin"), func() { kit.Select(c, &check.PluginID, options) })
		ui.Field(c, text("failure"), func() {
			kit.Select(c, &check.OnFailure, []kit.SelectOption{{Value: "allow", Label: text("allow")}, {Value: "skip", Label: text("skip")}, {Value: "block", Label: text("block")}})
		})
	}
	kit.InlineError(c, p.error)
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.Button(c, text("refresh"), icons.RefreshCw, p.loading).Clicked() {
			p.refresh(env, point)
		}
		if kit.Button(c, text("manage"), icons.Plug, false).Clicked() {
			p.manager = NewManager(env)
			p.manager.changed = func() { p.refresh(env, point) }
		}
	})
}

func (p *Picker) Dialogs(c *ui.Context) {
	if p.manager != nil {
		p.manager.View(c)
		if !p.manager.open {
			p.manager = nil
		}
	}
}

func (p *Picker) Close() {
	if p.manager != nil {
		p.manager.Close()
	}
}

type plugin struct {
	PluginID string `json:"pluginId"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Enabled  bool   `json:"enabled"`
	// HasSettings is a settings page of the plugin's own.
	HasSettings bool `json:"hasSettings"`
	// Pages are the plugin's panel and main view.
	Pages []struct {
		Page  string `json:"page"`
		Title string `json:"title"`
	} `json:"pages"`
}

type oauthAccount struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type Permission struct {
	Key        string         `json:"key"`
	LabelKey   string         `json:"labelKey"`
	DetailKey  string         `json:"detailKey"`
	Detail     string         `json:"detail"`
	LabelArgs  map[string]any `json:"labelArgs"`
	DetailArgs map[string]any `json:"detailArgs"`
}

type inspection struct {
	Token       string       `json:"token"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Update      bool         `json:"update"`
	Permissions []Permission `json:"permissions"`
}

type secret struct {
	Key, Label, Hint, Source string
	Saved                    bool
}

type taskPreferences struct {
	AgentKind      string          `json:"agentKind,omitempty"`
	ProviderID     string          `json:"providerId,omitempty"`
	Model          string          `json:"model,omitempty"`
	Effort         string          `json:"effort,omitempty"`
	FastMode       json.RawMessage `json:"fastMode,omitempty"`
	PermissionMode string          `json:"permissionMode,omitempty"`
	WorkingDir     string          `json:"workingDir,omitempty"`
}

func taskPreferencesJSON(value taskPreferences) string {
	data, _ := json.Marshal(value)
	return string(data)
}

// Manager uses native kit dialogs, with real package confirmation and shared
// unsaved/busy protection for credentials and the complete JSON parameter set.
type Manager struct {
	env                   *core.Env
	open, busy, loading   bool
	rows                  []plugin
	error                 string
	inspect               *inspection
	remove, configuration *plugin
	parameters, baseline  string
	secrets               []secret
	secretText            map[string]string
	secretClear           map[string]bool
	changed               func()
	// settings is a plugin's own settings page in an isolated web view.
	settings                      *globalpages.SettingsPage
	settingsName                  string
	accounts                      map[string][]oauthAccount
	taskPreferences, taskBaseline taskPreferences
	showTaskPreferences           bool
	providers                     []widgets.AIProvider
	modelsLoading                 bool
	backgroundTasks               []backgroundTask
	backgroundLoading             bool
	targetPlugin                  string
	targetCustom                  bool
	settingsPlugin                string
}

func NewManager(env *core.Env) *Manager {
	m := &Manager{env: env, open: true}
	m.refresh()
	return m
}
func (m *Manager) Dirty() bool {
	if m.configuration == nil {
		return false
	}
	if m.parameters != m.baseline || m.showTaskPreferences && taskPreferencesJSON(m.taskPreferences) != taskPreferencesJSON(m.taskBaseline) {
		return true
	}
	for _, value := range m.secretText {
		if value != "" {
			return true
		}
	}
	for _, value := range m.secretClear {
		if value {
			return true
		}
	}
	return false
}
func (m *Manager) discard() {
	m.parameters = m.baseline
	m.taskPreferences = m.taskBaseline
	m.taskPreferences.FastMode = append(json.RawMessage(nil), m.taskBaseline.FastMode...)
	m.secretText, m.secretClear = map[string]string{}, map[string]bool{}
}
func (m *Manager) Close() {
	m.open = false
	if m.settings != nil {
		m.settings.Close()
		m.settings = nil
	}
	m.env.Activity.Report("plugin-check-configuration", false, false, nil)
}
func (m *Manager) call(method string, params any, done func(json.RawMessage)) {
	if m.busy {
		return
	}
	m.busy = true
	core.Go(m.env, func() (json.RawMessage, error) { return m.env.Host.Call("chat.ghosts."+method, params) }, func(value json.RawMessage, err error) {
		if !m.open {
			return
		}
		m.busy = false
		if err != nil {
			m.error = err.Error()
			return
		}
		m.error = ""
		if done != nil {
			done(value)
		}
	})
}
func (m *Manager) refresh() {
	m.call("list", map[string]string{"language": i18n.Locale()}, func(value json.RawMessage) {
		if err := json.Unmarshal(value, &m.rows); err != nil {
			m.error = err.Error()
			return
		}
		m.openTarget()
	})
}

// OpenTarget selects this plugin's real native configuration/account card or
// its custom settings page after the current installed list has arrived.
func (m *Manager) OpenTarget(plugin string, custom bool) {
	m.targetPlugin, m.targetCustom = plugin, custom
	if !m.busy {
		m.openTarget()
	}
}
func (m *Manager) openTarget() {
	if m.targetPlugin == "" || m.busy {
		return
	}
	for _, row := range m.rows {
		if row.PluginID != m.targetPlugin {
			continue
		}
		m.targetPlugin = ""
		if m.targetCustom && row.HasSettings {
			m.openSettings(row)
		} else {
			m.configure(row)
		}
		return
	}
	m.error = text("missing", "id", m.targetPlugin)
}
func (m *Manager) TargetOpened(plugin string) bool {
	return m.open && (m.configuration != nil && m.configuration.PluginID == plugin || m.settings != nil && m.settingsPlugin == plugin)
}
func (m *Manager) Error() string { return m.error }
func (m *Manager) inspectPath(path string) {
	m.call("inspect", map[string]string{"path": path, "language": i18n.Locale()}, func(value json.RawMessage) {
		var result inspection
		if err := json.Unmarshal(value, &result); err != nil {
			m.error = err.Error()
		} else {
			m.inspect = &result
		}
	})
}
func (m *Manager) choosePackage() {
	core.Go(m.env, func() (string, error) {
		return m.env.Dialogs.PickFile(text("install"), core.Filter{Name: text("package"), Extensions: []string{"cindy"}})
	}, func(path string, err error) {
		if !m.open {
			return
		}
		if err != nil {
			m.error = err.Error()
		} else if path != "" {
			m.inspectPath(path)
		}
	})
}
func (m *Manager) configure(row plugin) {
	m.call("configuration", map[string]string{"pluginId": row.PluginID}, func(value json.RawMessage) {
		var result struct {
			Parameters          map[string]any  `json:"parameters"`
			Secrets             []secret        `json:"secrets"`
			TaskPreferences     taskPreferences `json:"taskPreferences"`
			ShowTaskPreferences bool            `json:"showTaskPreferences"`
		}
		if err := json.Unmarshal(value, &result); err != nil {
			m.error = err.Error()
			return
		}
		data, _ := json.MarshalIndent(result.Parameters, "", "  ")
		m.configuration, m.parameters, m.baseline, m.secrets = &row, string(data), string(data), result.Secrets
		m.taskBaseline, m.showTaskPreferences = result.TaskPreferences, result.ShowTaskPreferences
		m.discard()
		if m.showTaskPreferences {
			m.loadModels()
		}
		m.loadBackgroundTasks(row.PluginID)
	})
}
func (m *Manager) save() {
	var params map[string]any
	if json.Unmarshal([]byte(m.parameters), &params) != nil || params == nil {
		m.error = text("invalidJSON")
		return
	}
	values := map[string]any{}
	for key, value := range m.secretText {
		if value != "" {
			values[key] = value
		}
	}
	for key, clear := range m.secretClear {
		if clear {
			values[key] = nil
		}
	}
	row := *m.configuration
	sentParameters, sentPreferences := m.parameters, m.taskPreferences
	sentPreferences.FastMode = append(json.RawMessage(nil), sentPreferences.FastMode...)
	request := map[string]any{"pluginId": row.PluginID, "parameters": params, "secrets": values}
	if m.showTaskPreferences && taskPreferencesJSON(m.taskPreferences) != taskPreferencesJSON(m.taskBaseline) {
		request["taskPreferences"] = m.taskPreferences
	}
	m.call("configure", request, func(json.RawMessage) {
		m.baseline, m.taskBaseline = sentParameters, sentPreferences
		for key, value := range values {
			for i := range m.secrets {
				if m.secrets[i].Key == key {
					m.secrets[i].Saved = value != nil
				}
			}
			if submitted, ok := value.(string); ok && m.secretText[key] == submitted {
				delete(m.secretText, key)
			}
			if value == nil && m.secretClear[key] {
				delete(m.secretClear, key)
			}
		}
		// An asynchronous folder choice may arrive during the save. Only the
		// submitted snapshot becomes baseline; do not overwrite a newer draft.
		if !m.Dirty() {
			m.configure(row)
		}
	})
}
func translatePermission(key string, args map[string]any) string {
	values := []any{}
	for name, value := range args {
		values = append(values, name, value)
	}
	return i18n.T("ghostPermission."+key, values...)
}

// PermissionsKnown prevents an installation confirmation from omitting a
// capability whose explanation is unavailable in this client.
func PermissionsKnown(items []Permission) bool {
	for _, item := range items {
		if !i18n.Has("ghostPermission."+item.LabelKey) || item.DetailKey != "" && !i18n.Has("ghostPermission."+item.DetailKey) {
			return false
		}
	}
	return true
}

// PermissionDetails is shared by manual installation and Agent-requested
// installation, so both show the same fully interpolated permission facts.
func PermissionDetails(c *ui.Context, items []Permission) {
	for _, item := range items {
		kit.Strong(c, translatePermission(item.LabelKey, item.LabelArgs)).FillWidth()
		if item.DetailKey != "" {
			kit.Help(c, translatePermission(item.DetailKey, item.DetailArgs))
		}
		if item.Detail != "" {
			kit.Help(c, item.Detail)
		}
	}
}

func (m *Manager) View(c *ui.Context) {
	if !m.open {
		return
	}
	m.env.Activity.Report("plugin-check-configuration", m.Dirty(), m.busy, m.discard)
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: text("title"), Width: 720, Cancel: text("close"), Busy: m.busy, Error: m.error, Body: func() {
		if m.configuration != nil {
			m.configurationView(c)
			return
		}
		button := kit.Button(c, text("install"), icons.Plus, m.busy)
		if button.Clicked() {
			m.choosePackage()
		}
		widgets.DropArea(c, m.env, button, widgets.DropFiles, false, m.busy, func(paths []string, err error) {
			if err != nil {
				m.error = err.Error()
			} else if len(paths) > 0 {
				m.inspectPath(paths[0])
			}
		})
		if len(m.rows) == 0 && !m.busy {
			kit.Help(c, text("empty"))
		}
		for _, row := range m.rows {
			ui.Column(c.Key(row.PluginID)).FillWidth().Gap(6).Padding(10, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				kit.Strong(c, row.Name).FillWidth().SingleLine().Ellipsis("…").Tooltip(row.Name)
				kit.Caption(c, row.Version)
				enabled := row.Enabled
				if kit.Toggle(c, &enabled, text("enabledLabel", "name", row.Name), "").Disabled(m.busy).Changed() {
					m.call("enabled", map[string]any{"pluginId": row.PluginID, "enabled": enabled}, func(json.RawMessage) {
						m.refresh()
						if m.changed != nil {
							m.changed()
						}
					})
				}
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					if kit.Button(c, text("configure"), icons.Settings2, false).Disabled(m.busy).Clicked() {
						m.configure(row)
					}
					for _, page := range row.Pages {
						if kit.Button(c.Key(page.Page), page.Title, icons.PanelRightOpen, false).Disabled(m.busy).Clicked() {
							m.openPage(row, page.Page, page.Title)
						}
					}
					if row.HasSettings && kit.Button(c, text("settings"), icons.Settings2, false).Disabled(m.busy).Clicked() {
						m.openSettings(row)
					}
					if kit.Button(c, text("remove"), icons.Trash2, false).Disabled(m.busy).Clicked() {
						copy := row
						m.remove = &copy
					}
				})
			})
		}
	}})
	if !open {
		m.env.RequestChange(m.Close)
	}
	if m.settings != nil {
		open := true
		kit.Dialog(c.Key("settings"), &open, kit.DialogOptions{Title: m.settingsName, Width: 760, Cancel: text("close"), Body: func() {
			m.settings.View(c)
		}})
		if !open {
			m.settings.Close()
			m.settings = nil
		}
	}
	if m.inspect != nil {
		m.confirmView(c)
	}
	if m.remove != nil {
		row := *m.remove
		open := true
		if kit.Dialog(c, &open, kit.DialogOptions{Title: text("removeTitle", "name", row.Name), Message: text("removeHint"), Confirm: text("remove"), Cancel: text("cancel"), Busy: m.busy, Error: m.error, Danger: true}) {
			m.call("remove", map[string]any{"pluginId": row.PluginID, "confirmed": true}, func(json.RawMessage) {
				m.remove = nil
				m.refresh()
				if m.changed != nil {
					m.changed()
				}
			})
		}
		if !open {
			m.remove = nil
		}
	}
}

// openSettings starts the plugin's sandbox and shows its own page, which reaches
// only its same-origin settings endpoints.
func (m *Manager) openSettings(row plugin) {
	preview, err := globalpages.NewSettingsPage(m.env, row.PluginID)
	if err != nil {
		m.error = err.Error()
		return
	}
	if m.settings != nil {
		m.settings.Close()
	}
	m.settings, m.settingsName, m.settingsPlugin = preview, row.Name, row.PluginID
}

// Panels/main views use their declared native containers and ordinary routes,
// not a second fixed-size preview dialog. The existing guard runs before the
// management page is closed, so cancellation preserves its pending draft.
func (m *Manager) openPage(row plugin, page, title string) {
	m.env.RequestChange(func() {
		m.Close()
		globalpages.For(m.env).Open(row.PluginID, page)
	})
}

// oauthView lists a plugin's signed-in accounts. Tokens stay in the plugin host.
func (m *Manager) oauthView(c *ui.Context, item secret) {
	row := *m.configuration
	if m.accounts == nil {
		m.accounts = map[string][]oauthAccount{}
	}
	action := func(name, account string, done func(json.RawMessage)) {
		m.call("account-action", map[string]string{"pluginId": row.PluginID, "action": name, "key": item.Key, "accountId": account}, done)
	}
	refresh := func(json.RawMessage) {
		action("oauth-list", "", func(value json.RawMessage) {
			var listed struct {
				Accounts []oauthAccount `json:"accounts"`
			}
			if json.Unmarshal(value, &listed) == nil {
				m.accounts[item.Key] = listed.Accounts
			}
		})
	}
	if _, known := m.accounts[item.Key]; !known {
		m.accounts[item.Key] = nil
		refresh(nil)
	}
	ui.Field(c, item.Label, func() {
		for _, account := range m.accounts[item.Key] {
			ui.Row(c.Key(account.ID)).FillWidth().Gap(8).Children(func() {
				kit.Caption(c, account.Label+" · "+account.Status).Grow(1).MinWidth(0)
				if kit.GhostButton(c, text("disconnect"), icons.Unplug, false).Disabled(m.busy).Clicked() {
					action("oauth-disconnect", account.ID, refresh)
				}
			})
		}
		if kit.Button(c, text("connect"), icons.Plus, false).Disabled(m.busy).Clicked() {
			action("oauth-connect", "", refresh)
		}
	})
}

func (m *Manager) configurationView(c *ui.Context) {
	if kit.GhostButton(c, text("back"), icons.ArrowLeft, false).Disabled(m.busy).Clicked() {
		m.env.RequestChange(func() { m.configuration = nil })
		return // A clean form navigates synchronously; do not render its old state.
	}
	kit.Strong(c, m.configuration.Name)
	globalpages.For(m.env).ManagementControls(c, m.configuration.PluginID)
	m.backgroundTaskFields(c)
	if m.showTaskPreferences {
		m.taskFields(c)
	}
	ui.Field(c, text("parameters"), func() { ui.TextArea(c, &m.parameters).Height(180).FillWidth() })
	for _, item := range m.secrets {
		if item.Source == "oauth" {
			m.oauthView(c, item)
			continue
		}
		if item.Source != "user" && item.Source != "gh-cli" {
			kit.Help(c, item.Label+" · "+text("managedSecret"))
			continue
		}
		value := m.secretText[item.Key]
		ui.Field(c, item.Label, func() {
			placeholder := text("secretMissing")
			if item.Saved {
				placeholder = text("secretSaved")
			}
			if ui.TextInput(c, &value).Password().FillWidth().Label(item.Label).Placeholder(placeholder).Changed() {
				m.secretText[item.Key] = value
			}
		})
		if strings.TrimSpace(item.Hint) != "" {
			kit.Help(c, item.Hint)
		}
		clear := m.secretClear[item.Key]
		if kit.Toggle(c, &clear, text("clearSecret"), "").Changed() {
			m.secretClear[item.Key] = clear
		}
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, text("save"), icons.Save, m.busy).Disabled(!m.Dirty()).Clicked() {
			m.save()
		}
		if kit.Button(c, text("undo"), icons.Undo2, false).Disabled(m.busy || !m.Dirty()).Clicked() {
			m.discard()
		}
	})
}
func (m *Manager) confirmView(c *ui.Context) {
	value := m.inspect
	key := "confirmTitle"
	if value.Update {
		key = "updateTitle"
	}
	unsupported := !PermissionsKnown(value.Permissions)
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: text(key, "name", value.Name), Message: text("confirmHint"), Confirm: text("confirm"), Cancel: text("cancel"), Width: 680, Busy: m.busy, Error: m.error, ConfirmDisabled: unsupported, Body: func() {
		kit.Caption(c, value.Version)
		PermissionDetails(c, value.Permissions)
		if unsupported {
			kit.Help(c, text("unsupportedPermission"))
		}
	}}) {
		m.call("install", map[string]any{"token": value.Token, "confirmed": true}, func(json.RawMessage) {
			m.inspect = nil
			m.refresh()
			if m.changed != nil {
				m.changed()
			}
		})
	}
	if !open {
		m.inspect = nil
	}
}

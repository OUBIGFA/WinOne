package selection

import (
	"errors"
	"strings"
	"winone/internal/resultwindow"
)

// Preferences contain only model references, never connections or credentials.
// All AI actions are routed by the shared service after a toolbar action.
type Preferences struct {
	ToolbarVersion int                            `json:"toolbarVersion,omitempty"`
	Actions        []resultwindow.SelectionAction `json:"actions,omitempty"`
	SchemaVersion  int                            `json:"schemaVersion"`
	Revision       uint64                         `json:"revision"`
	Automatic      bool                           `json:"automatic"`
	Keyboard       bool                           `json:"keyboard"`
	ExcludedApps   []string                       `json:"excludedApps"`
}

func Defaults() Preferences {
	return Preferences{SchemaVersion: 1, ToolbarVersion: 1, Automatic: true, Keyboard: true, ExcludedApps: []string{}, Actions: resultwindow.DefaultSelectionActions()}
}

func (p Preferences) EffectiveActions() []resultwindow.SelectionAction {
	if p.Actions == nil {
		return resultwindow.DefaultSelectionActions()
	}
	actions := resultwindow.UpgradeBuiltinActions(p.Actions)
	// Move the built-in copy action once for older toolbars. After settings
	// have been read/saved at version 1, all explicit user ordering wins.
	if p.ToolbarVersion == 0 {
		for index, action := range actions {
			if action.ID == "copy" {
				actions = append(append(actions[:index:index], actions[index+1:]...), action)
				break
			}
		}
	}
	return actions
}
func (p Preferences) Validate() error {
	if p.SchemaVersion != 1 || p.ToolbarVersion < 0 || p.ToolbarVersion > 1 {
		return errors.New("不支持的划词设置版本")
	}
	if len(p.ExcludedApps) > 100 {
		return errors.New("排除应用数量过多")
	}
	for _, app := range p.ExcludedApps {
		if strings.TrimSpace(app) != app || app == "" || len(app) > 260 || strings.ContainsAny(app, "/\\:\x00\r\n") || !strings.HasSuffix(strings.ToLower(app), ".exe") {
			return errors.New("排除应用请填写程序文件名，例如 notepad.exe")
		}
	}
	return resultwindow.ValidateActions(p.EffectiveActions())
}

func (p Preferences) Excludes(name string) bool {
	for _, app := range p.ExcludedApps {
		if strings.EqualFold(name, app) {
			return true
		}
	}
	return false
}

// Package quickassistant owns preferences for the on-demand AI chat window.
package quickassistant

import (
	"errors"
	"winone/internal/resultwindow"
)

type Preferences struct {
	SchemaVersion int                            `json:"schemaVersion"`
	Revision      uint64                         `json:"revision"`
	AlwaysOnTop   bool                           `json:"alwaysOnTop"`
	ShowReasoning *bool                          `json:"showReasoning,omitempty"`
	Actions       []resultwindow.SelectionAction `json:"actions"`
}

func Defaults() Preferences {
	return Preferences{SchemaVersion: 1, Actions: resultwindow.QuickAssistantActions(nil)}
}
func (p Preferences) EffectiveActions() []resultwindow.SelectionAction {
	return resultwindow.QuickAssistantActions(p.Actions)
}
func (p Preferences) Validate() error {
	if p.SchemaVersion != 1 {
		return errors.New("不支持的快捷助手设置版本")
	}
	return resultwindow.ValidateActions(p.EffectiveActions())
}

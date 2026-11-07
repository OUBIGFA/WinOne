// Package quickassistant owns preferences for the on-demand AI chat window.
package quickassistant

import (
	"errors"
	"winone/internal/resultwindow"
)

type Preferences struct {
	LearningVersion int                            `json:"learningVersion,omitempty"`
	SchemaVersion   int                            `json:"schemaVersion"`
	Revision        uint64                         `json:"revision"`
	AlwaysOnTop     bool                           `json:"alwaysOnTop"`
	ShowReasoning   *bool                          `json:"showReasoning,omitempty"`
	Actions         []resultwindow.SelectionAction `json:"actions"`
}

func Defaults() Preferences {
	return Preferences{SchemaVersion: 1, LearningVersion: 1, Actions: resultwindow.QuickAssistantActions(nil)}
}
func (p Preferences) EffectiveActions() []resultwindow.SelectionAction {
	actions := resultwindow.QuickAssistantActions(p.Actions)
	if p.LearningVersion == 0 {
		actions = resultwindow.AddLearningAction(actions)
	}
	return actions
}
func (p Preferences) Validate() error {
	if p.SchemaVersion != 1 || p.LearningVersion < 0 || p.LearningVersion > 1 {
		return errors.New("不支持的快捷助手设置版本")
	}
	return resultwindow.ValidateActions(p.EffectiveActions())
}

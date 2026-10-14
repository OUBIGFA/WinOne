package resultui

import (
	"github.com/egoist/mygo/ui"
	"winone/internal/ai"
)

func (p *Panel) reasoningVisible() bool {
	if p.Source == "quick-assistant" {
		return p.QuickReasoning == nil || *p.QuickReasoning
	}
	return p.Configuration.ReasoningVisible()
}

// Result translation and the conversation have independent model choices.
// The actual completed route remains available for conversation history.
func (p *Panel) translationModel() ai.Selection {
	if p.TranslationSelection != nil {
		return *p.TranslationSelection
	}
	if !p.TranslationModelChanged && p.ResultSelection.Provider != "" {
		return p.ResultSelection
	}
	if p.Source == "selection" {
		if p.ActionSelection.Provider != "" {
			return p.ActionSelection
		}
		return p.Configuration.Selected("default")
	}
	if p.Configuration.Engine == "google" {
		return ai.Selection{}
	}
	return p.Configuration.Selected("translate")
}
func (p *Panel) chooseTranslationModel(selection *ai.Selection) {
	previous := p.translationModel()
	p.TranslationSelection = selection
	p.TranslationModelChanged = true
	p.TranslationPickerOpen = false
	if previous == p.translationModel() {
		return
	}
	if p.Configuration.RetranslateOnModelChange() && !p.Busy && p.Actions.RetryResult != nil {
		p.Actions.RetryResult()
	}
}
func (p *Panel) translationModelPicker(c *ui.Context, t *ui.Theme) { p.modelPickerFor(c, t, true) }
func (c *Controller) translationParameters(values map[string]any) map[string]any {
	values["session"] = c.Initial.ID
	if c.Panel.TranslationSelection != nil {
		values["selection"] = *c.Panel.TranslationSelection
	} else if c.Initial.Source == "selection" {
		selection := c.Panel.ActionSelection
		if selection.Provider == "" {
			selection = c.Panel.Configuration.Selected("default")
		}
		values["selection"] = selection
	}
	return values
}

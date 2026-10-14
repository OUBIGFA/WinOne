package resultwindow

import (
	"strings"
	"winone/internal/ai"
)

// SelectionAction is a saved user action snapshot shared by settings, the
// transient toolbar and the existing result controller. Order is array order.
type SelectionAction struct {
	Selection   ai.Selection `json:"selection,omitzero"` // Reference to the shared enabled model collection.
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	EnglishName string       `json:"englishName,omitempty"`
	Icon        string       `json:"icon"`
	Kind        string       `json:"kind"`
	Prompt      string       `json:"prompt,omitempty"`
	Enabled     bool         `json:"enabled"`
}

// Keep this exact prior prompt text for migrations. It also supplies unchanged
// names, icons and output rules to the current defaults below.
func previousSelectionActions() []SelectionAction {
	return []SelectionAction{
		{ID: "translate", Name: "翻译", EnglishName: "Translate", Icon: "translate", Kind: "translate", Prompt: ai.DefaultTranslationPrompt, Enabled: true},
		{ID: "explain", Name: "解释", EnglishName: "Explain", Icon: "explain", Kind: "ai", Prompt: "Explain the supplied content. State the main idea first. Define necessary terms in common words. Use short sentences with one main idea each. Use one term for each concept. Give an example only when it makes the meaning clearer. Separate stated facts from assumptions. State what the content does not establish. Use the user's language; if it is not specified, use the language of the supplied content. Return only the explanation.", Enabled: true},
		{ID: "summarize", Name: "总结", EnglishName: "Summarize", Icon: "summarize", Kind: "ai", Prompt: "Summarize the supplied content. State the main point first. Give the essential supporting facts in a short list when useful. Keep names, numbers, units, conditions, exceptions, and uncertainty that affect the meaning. Do not add facts or conclusions that the content does not support. Use short sentences and one term for each concept. Use the user's language; if it is not specified, use the language of the supplied content. Return only the summary.", Enabled: true},
		{ID: "polish", Name: "润色", EnglishName: "Polish", Icon: "polish", Kind: "ai", Prompt: "Improve the wording of the supplied content. Keep its meaning, language, tone, and level of certainty. Use common words, active voice, and short sentences. Keep one main idea in each sentence. Use one term for each concept. Remove repetition without removing necessary conditions. Keep names, facts, numbers, units, links, paragraph order, and formatting. Do not add claims or explanations. Return only the revised content.", Enabled: true},
		{ID: "code", Name: "代码", EnglishName: "Code", Icon: "code", Kind: "ai", Prompt: "Help with the supplied code or programming question. State the task or problem first. Explain the cause using the available evidence. If information is missing, state the assumption or ask one specific question. Give the smallest useful correction. Keep the programming language, public interfaces, and existing behavior unless the task requires a change. Put code in fenced blocks with the language name. Explain how to check the result. Do not claim that code was run or tested unless a tool result proves it. Use short sentences and stable technical terms. Use the user's language.", Enabled: true},
		{ID: "copy", Name: "复制", EnglishName: "Copy", Icon: "copy", Kind: "copy", Enabled: true},
	}
}

const replyLanguageTemplate = "Use {replyLanguage}, the current WinOne interface language, for all natural-language output, regardless of the language of the supplied content. Keep code and identifiers unchanged unless the task requires changing them."

func ReplyLanguageInstruction(locale string) string {
	language := "Simplified Chinese"
	if locale == "en-US" {
		language = "English"
	}
	return strings.ReplaceAll(replyLanguageTemplate, "{replyLanguage}", language)
}

func DefaultSelectionActions() []SelectionAction {
	actions := previousSelectionActions()
	for i := range actions {
		action := &actions[i]
		if action.Kind == "copy" {
			continue
		}
		if action.Kind == "translate" {
			// Translation retains smart Chinese/English direction and explicit
			// targets; reply language applies to the other assistant actions.
			continue
		}
		// Remove the previous source-language fallback, including Polish's
		// language-preservation clause, so the instructions do not conflict.
		action.Prompt = strings.ReplaceAll(action.Prompt, "Use the user's language; if it is not specified, use the language of the supplied content. ", "")
		action.Prompt = strings.ReplaceAll(action.Prompt, " Use the user's language.", "")
		action.Prompt = strings.ReplaceAll(action.Prompt, "Keep its meaning, language, tone, and level of certainty.", "Keep its meaning, tone, and level of certainty.")
		action.Prompt += " " + replyLanguageTemplate
	}
	return actions
}

// QuickAssistantActions shares the same capabilities but omits the redundant
// toolbar Copy action. Each chat reply still has its own copy button.
func QuickAssistantActions(actions []SelectionAction) []SelectionAction {
	if actions == nil {
		actions = DefaultSelectionActions()
	}
	actions = UpgradeBuiltinActions(actions)
	result := make([]SelectionAction, 0, len(actions))
	for _, action := range actions {
		if action.Kind != "copy" {
			result = append(result, action)
		}
	}
	return result
}

// Upgrade only exact legacy defaults. Names, ordering, model choices and
// user-authored prompts are independent settings and must survive an upgrade.
func UpgradeBuiltinActions(actions []SelectionAction) []SelectionAction {
	result := append([]SelectionAction(nil), actions...)
	legacy := map[string]string{
		"explain":   "Explain the selected content clearly and concisely. Match the user's language.",
		"summarize": "Summarize the selected content, retaining its main facts. Match the user's language.",
		"polish":    "Improve the wording of the selected content while preserving its meaning and language. Return the polished version.",
		"code":      "Help with the selected code or programming question. Explain concrete issues and provide a corrected example when useful. Use Markdown code fences.",
	}
	defaults := DefaultSelectionActions()
	previous := map[string]string{}
	for _, action := range previousSelectionActions() {
		previous[action.ID] = action.Prompt
	}
	for i, action := range result {
		for _, current := range defaults {
			if action.ID == current.ID && action.Kind == current.Kind && action.Kind != "copy" && ((action.Kind == "translate" && action.Prompt == "") || action.Prompt == legacy[action.ID] || action.Prompt == previous[action.ID]) {
				result[i].Prompt = current.Prompt
			}
		}
	}
	return result
}

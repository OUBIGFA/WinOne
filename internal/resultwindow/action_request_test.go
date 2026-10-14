package resultwindow

import (
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestBuiltinActionsDefaultToWinOneReplyLanguage(t *testing.T) {
	for _, action := range DefaultSelectionActions() {
		if action.Kind == "copy" {
			continue
		}
		t.Run(action.ID, func(t *testing.T) {
			_, params, err := ActionRequest(action, "An English paragraph.", "auto")
			if err != nil {
				t.Fatal(err)
			}
			field := "system"
			if action.Kind == "translate" {
				if params["target"] != "auto" || params["prompt"] != ai.DefaultTranslationPrompt {
					t.Error("smart Chinese/English translation was changed", params)
				}
				return
			}
			prompt, _ := params[field].(string)
			if !strings.Contains(prompt, "Simplified Chinese") {
				t.Fatal("default action did not request the resolved interface language", prompt)
			}
		})
	}
}

func TestActionsKeepTheSameInputAndCustomPrompt(t *testing.T) {
	text := "Original input, not an earlier answer"
	actions := DefaultSelectionActions()
	method, translation, err := ActionRequest(actions[0], text, "en")
	if err != nil || method != "ai.translate.start" || translation["text"] != text {
		t.Fatal(method, translation, err)
	}
	method, explanation, err := ActionRequest(actions[1], text, "en")
	if err != nil || method != "ai.generate.start" || explanation["messages"].([]ai.Message)[0].Content != "Selected content:\n"+text {
		t.Fatal(method, explanation, err)
	}
	if _, _, err = ActionRequest(actions[1], " \n", ""); err == nil {
		t.Fatal("blank action accepted")
	}
}

func TestAssistantLanguageExpansionRespectsExplicitTargetsAndCustomInstructions(t *testing.T) {
	actions := DefaultSelectionActions()
	for _, locale := range []string{"zh-CN", "en-US"} {
		wantLanguage := "Simplified Chinese"
		if locale == "en-US" {
			wantLanguage = "English"
		}
		_, translated, err := ActionRequest(actions[0], "mixed language text", "auto", locale)
		if err != nil || translated["target"] != "auto" || translated["prompt"] != ai.DefaultTranslationPrompt {
			t.Fatal(translated, err)
		}
		_, explicit, err := ActionRequest(actions[0], "mixed language text", "ja", locale)
		if err != nil || explicit["target"] != "ja" {
			t.Fatal("explicit translation target changed", explicit, err)
		}
		for _, action := range actions[1:5] {
			_, params, err := ActionRequest(action, "mixed language text", "auto", locale)
			if err != nil || !strings.Contains(params["system"].(string), wantLanguage) || strings.Contains(params["system"].(string), "{replyLanguage}") {
				t.Fatal(params, err)
			}
		}
	}
	custom := SelectionAction{ID: "my-action", Kind: "ai", Prompt: "Reply in Japanese."}
	_, params, err := ActionRequest(custom, "source text", "auto", "zh-CN")
	if err != nil || !strings.HasPrefix(params["system"].(string), "Reply in Japanese.") || strings.Contains(params["system"].(string), "Simplified Chinese") {
		t.Fatal("custom instruction was overwritten", params, err)
	}
}

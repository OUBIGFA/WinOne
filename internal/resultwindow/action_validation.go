package resultwindow

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var selectionProviderID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var actionID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ValidateActions is shared by both assistants and profile import.
func ValidateActions(actions []SelectionAction) error {

	if len(actions) < 1 || len(actions) > 24 {
		return errors.New("工具栏需要 1 至 24 个功能")
	}
	seen := map[string]bool{}
	enabled := 0
	for _, action := range actions {
		if action.Selection.Provider != "" || action.Selection.Model != "" {
			if action.Kind == "copy" || !selectionProviderID.MatchString(action.Selection.Provider) || strings.TrimSpace(action.Selection.Model) != action.Selection.Model || action.Selection.Model == "" || len(action.Selection.Model) > 256 {
				return errors.New("请选择供应商及其模型")
			}
		}
		if !actionID.MatchString(action.ID) || seen[action.ID] {
			return errors.New("工具栏功能标识无效或重复")
		}
		seen[action.ID] = true
		if strings.TrimSpace(action.Name) == "" || utf8.RuneCountInString(action.Name) > 40 || utf8.RuneCountInString(action.EnglishName) > 60 {
			return errors.New("请填写简短的功能名称")
		}
		switch action.Icon {
		case "copy", "translate", "explain", "summarize", "polish", "code", "conversation", "service", "book", "idea", "search", "document", "check", "write", "mail", "globe", "learn", "calculate", "terminal", "braces", "tasks", "text", "case", "wand", "astroid":
		default:
			return errors.New("请选择支持的功能图标")
		}
		switch action.Kind {
		case "copy", "translate":
			if len(action.Prompt) > 16000 {
				return errors.New("AI 指令超过 16000 字节")
			}
		case "ai":
			if strings.TrimSpace(action.Prompt) == "" || len(action.Prompt) > 16000 {
				return errors.New("AI 指令不能为空或超过 16000 字节")
			}
		default:
			return errors.New("不支持的工具栏功能类型")
		}
		if action.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return errors.New("请至少显示一个工具栏功能")
	}
	return nil
}

package captureprefs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"winone/internal/hotkeys"
)

// Canvas actions are local to the capture window; bare letters never register
// globally. The same catalog generates the native command routing.
type CanvasAction struct{ ID, Zh, En, Key, Command, Tool string }

//go:embed shortcuts.json
var canvasCatalog []byte
var CanvasActions = func() []CanvasAction {
	var actions []CanvasAction
	if err := json.Unmarshal(canvasCatalog, &actions); err != nil {
		panic(err)
	}
	return actions
}()

func CanvasShortcut(values map[string]string, id string) string {
	if key, ok := values[id]; ok {
		return key
	}
	for _, action := range CanvasActions {
		if action.ID == id {
			return action.Key
		}
	}
	return ""
}

func ValidateCanvasShortcuts(values map[string]string) error {
	known := map[string]bool{}
	used := map[hotkeys.Shortcut]string{}
	for _, action := range CanvasActions {
		known[action.ID] = true
		key := CanvasShortcut(values, action.ID)
		if key == "" {
			continue
		}
		normalized := strings.ToUpper(key)
		// Parse with an artificial modifier for local single-key actions.
		if !strings.Contains(normalized, "+") {
			normalized = "Ctrl+" + normalized
		}
		parsed, err := hotkeys.Parse(normalized)
		if err != nil {
			return fmt.Errorf("%s：%w", action.Zh, err)
		}
		if !strings.Contains(key, "+") {
			parsed.Modifiers = 0
		}
		if parsed.Modifiers&8 != 0 {
			return fmt.Errorf("截图内快捷键不使用 Win 键")
		}
		if other, ok := used[parsed]; ok {
			return fmt.Errorf("%s与%s使用了相同的截图快捷键", other, action.Zh)
		}
		used[parsed] = action.Zh
	}
	for id := range values {
		if !known[id] {
			return fmt.Errorf("未知截图快捷键：%s", id)
		}
	}
	return nil
}

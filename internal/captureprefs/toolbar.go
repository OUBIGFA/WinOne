package captureprefs

import (
	"encoding/json"
	"errors"
)

// Nil uses the shipped layout; an empty list intentionally keeps every tool in More.
func ValidateToolbar(tools []string) error {
	allowed := map[string]bool{"select": true, "crop": true, "rectangle": true, "ellipse": true, "arrow": true, "pen": true, "cover": true, "text": true, "mosaic": true, "blur": true}
	seen := map[string]bool{}
	for _, tool := range tools {
		if !allowed[tool] || seen[tool] {
			return errors.New("截图工具栏包含未知或重复工具")
		}
		seen[tool] = true
	}
	return nil
}

func DecodeToolbar(raw json.RawMessage) ([]string, error) {
	var tools []string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, err
		}
	}
	return tools, ValidateToolbar(tools)
}

// Package hotkeys owns global shortcut validation and Windows registration.
package hotkeys

import (
	"fmt"
	"strconv"
	"strings"
)

type Shortcut struct{ Modifiers, Key uint32 }

// Parse accepts a deliberately small vocabulary shared with the settings UI.
// Empty means disabled; bare character keys are rejected to preserve normal typing.
func Parse(value string) (Shortcut, error) {
	if strings.TrimSpace(value) == "" {
		return Shortcut{}, nil
	}
	var result Shortcut
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(value)), "+")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		var bit uint32
		switch part {
		case "CTRL":
			bit = 2
		case "ALT":
			bit = 1
		case "SHIFT":
			bit = 4
		case "WIN":
			bit = 8
		}
		if bit != 0 {
			if result.Modifiers&bit != 0 {
				return Shortcut{}, fmt.Errorf("修饰键重复")
			}
			result.Modifiers |= bit
			continue
		}
		if result.Key != 0 {
			return Shortcut{}, fmt.Errorf("快捷键只能包含一个主键")
		}
		if len(part) == 1 && (part[0] >= 'A' && part[0] <= 'Z' || part[0] >= '0' && part[0] <= '9') {
			result.Key = uint32(part[0])
			continue
		}
		if strings.HasPrefix(part, "F") {
			n, err := strconv.Atoi(part[1:])
			if err == nil && n >= 1 && n <= 24 {
				result.Key = uint32(0x70 + n - 1)
				continue
			}
		}
		if len(part) == 4 && strings.HasPrefix(part, "NUM") && part[3] >= '0' && part[3] <= '9' {
			result.Key = 0x60 + uint32(part[3]-'0')
			continue
		}
		return Shortcut{}, fmt.Errorf("主键须为字母、数字或 F1–F24")
	}
	if result.Key == 0 || result.Modifiers == 0 {
		return Shortcut{}, fmt.Errorf("快捷键须包含修饰键和主键")
	}
	if result.Key == 0x7b {
		return Shortcut{}, fmt.Errorf("F12 由 Windows 调试器保留，请换一个键")
	}
	return result, nil
}

type Binding struct {
	ID       string `json:"id"`
	Shortcut string `json:"shortcut"`
}
type Status struct {
	Binding
	Registered bool   `json:"registered"`
	Error      string `json:"error,omitempty"`
}

func validate(bindings []Binding) error {
	ids := map[string]bool{}
	keys := map[Shortcut]bool{}
	if len(bindings) > 32 {
		return fmt.Errorf("快捷键数量过多")
	}
	for _, b := range bindings {
		if b.ID == "" || ids[b.ID] {
			return fmt.Errorf("快捷键操作标识重复或为空")
		}
		ids[b.ID] = true
		key, err := Parse(b.Shortcut)
		if err != nil {
			return fmt.Errorf("%s：%w", b.ID, err)
		}
		if key.Key != 0 {
			if keys[key] {
				return fmt.Errorf("多个操作不能使用相同的快捷键")
			}
			keys[key] = true
		}
	}
	return nil
}

// ValidateBindings checks a pending configuration without registering it.
func ValidateBindings(bindings []Binding) error { return validate(bindings) }

//go:build windows

package interconnect

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"winone/internal/configjson"
	"winone/internal/settings"
)

// ChatPreferences is independent of transfer settings and room credentials.
// A transfer-settings draft must never overwrite a newly chosen nickname.
type ChatPreferences struct {
	Nickname string `json:"nickname"`
}

func normalizeNickname(value string) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 80 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", errors.New("个人昵称最多 80 个字符，不能包含换行或控制字符")
	}
	return value, nil
}

func (m *Manager) ChatPreferences() (ChatPreferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var value ChatPreferences
	data, err := os.ReadFile(filepath.Join(filepath.Dir(m.data), "chat.json"))
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err = configjson.DecodeObject(data, &value); err != nil {
		return value, err
	}
	value.Nickname, err = normalizeNickname(value.Nickname)
	return value, err
}

func (m *Manager) SaveChatPreferences(value ChatPreferences) (ChatPreferences, error) {
	var err error
	value.Nickname, err = normalizeNickname(value.Nickname)
	if err != nil {
		return ChatPreferences{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ChatPreferences{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// WriteAtomic persists the complete new value before acknowledging the UI.
	err = settings.WriteAtomic(filepath.Join(filepath.Dir(m.data), "chat.json"), data)
	return value, err
}

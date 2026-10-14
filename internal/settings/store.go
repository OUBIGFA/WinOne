// Package settings owns global preferences and rejects stale client updates.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"winone/internal/hotkeys"
)

var ErrConflict = errors.New("settings changed; reload before saving")

// Settings contains only global preferences; feature settings belong to their modules.
type Settings struct {
	SchemaVersion          int                 `json:"schemaVersion"`
	Revision               uint64              `json:"revision"`
	Theme                  string              `json:"theme"`
	Language               string              `json:"language,omitempty"`
	DefaultPage            string              `json:"defaultPage,omitempty"`
	OpenShortcut           *string             `json:"openShortcut,omitempty"`
	CaptureShortcut        string              `json:"captureShortcut"`
	OCRShortcut            *string             `json:"ocrShortcut,omitempty"`
	TranslateShortcut      *string             `json:"translateShortcut,omitempty"`
	QuickAssistantShortcut *string             `json:"quickAssistantShortcut,omitempty"`
	SelectionShortcut      *string             `json:"selectionShortcut,omitempty"`
	PinShortcut            string              `json:"pinShortcut"`
	Startup                *StartupPreferences `json:"startup,omitempty"`
}

// Startup belongs to WinOne, not to a module. Nil is a legacy document which
// the configuration store migrates from the old WinTray compatibility fields.
type StartupPreferences struct {
	RunAtLogon          bool `json:"runAtLogon"`
	StartMinimized      bool `json:"startMinimized"`
	PreferAdministrator bool `json:"preferAdministrator,omitempty"`
	// Nil adopts the current default without overwriting a legacy preference.
	KeepUIRunning *bool `json:"keepUIRunning,omitempty"`
}

func copySettings(value Settings) Settings {
	if value.OpenShortcut != nil {
		key := *value.OpenShortcut
		value.OpenShortcut = &key
	}
	if value.QuickAssistantShortcut != nil {
		key := *value.QuickAssistantShortcut
		value.QuickAssistantShortcut = &key
	}
	if value.SelectionShortcut != nil {
		key := *value.SelectionShortcut
		value.SelectionShortcut = &key
	}
	if value.OCRShortcut != nil {
		key := *value.OCRShortcut
		value.OCRShortcut = &key
	}
	if value.TranslateShortcut != nil {
		key := *value.TranslateShortcut
		value.TranslateShortcut = &key
	}
	if value.Startup != nil {
		startup := *value.Startup
		if startup.KeepUIRunning != nil {
			keep := *startup.KeepUIRunning
			startup.KeepUIRunning = &keep
		}
		value.Startup = &startup
	}
	return value
}

// Missing fields in an older configuration adopt the new defaults; an
// explicitly empty value remains disabled after saves and profile imports.
func (s Settings) OCRBinding() string {
	if s.OCRShortcut == nil {
		other := ""
		if s.TranslateShortcut != nil {
			other = *s.TranslateShortcut
		}
		return availableDefault("Shift+C", s.CaptureShortcut, s.PinShortcut, other, explicitShortcut(s.SelectionShortcut), explicitShortcut(s.QuickAssistantShortcut), explicitShortcut(s.OpenShortcut))
	}
	return *s.OCRShortcut
}
func (s Settings) TranslateBinding() string {
	if s.TranslateShortcut == nil {
		return availableDefault("Alt+Shift+A", s.CaptureShortcut, s.PinShortcut, s.OCRBinding(), explicitShortcut(s.SelectionShortcut), explicitShortcut(s.QuickAssistantShortcut), explicitShortcut(s.OpenShortcut))
	}
	return *s.TranslateShortcut
}

func (s Settings) SelectionBinding() string {
	if s.SelectionShortcut != nil {
		return *s.SelectionShortcut
	}
	return availableDefault("Alt+Shift+S", s.CaptureShortcut, s.PinShortcut, s.OCRBinding(), s.TranslateBinding(), explicitShortcut(s.QuickAssistantShortcut), explicitShortcut(s.OpenShortcut))
}

// Explicit user choices, including disabled bindings, always take precedence.
func explicitShortcut(value *string) string {
	if value != nil {
		return *value
	}
	return ""
}
func (s Settings) QuickAssistantBinding() string {
	if s.QuickAssistantShortcut != nil {
		return *s.QuickAssistantShortcut
	}
	return availableDefault("Ctrl+Shift+Q", s.CaptureShortcut, s.PinShortcut, s.OCRBinding(), s.TranslateBinding(), s.SelectionBinding(), explicitShortcut(s.OpenShortcut))
}

// A newly introduced default must not steal an older user-selected shortcut.
func (s Settings) OpenBinding() string {
	if s.OpenShortcut != nil {
		return *s.OpenShortcut
	}
	return availableDefault("Alt+A", s.CaptureShortcut, s.PinShortcut, s.OCRBinding(), s.TranslateBinding(), s.SelectionBinding(), s.QuickAssistantBinding())
}

func availableDefault(preferred string, existing ...string) string {
	key, _ := hotkeys.Parse(preferred)
	for _, value := range existing {
		parsed, err := hotkeys.Parse(value)
		if err == nil && key == parsed {
			return ""
		}
	}
	return preferred
}

// Expose the same effective defaults that registration uses, without writing
// an older configuration merely because its settings page was opened.
func (s Settings) ResolvedShortcuts() Settings {
	value := copySettings(s)
	ocr, translate := s.OCRBinding(), s.TranslateBinding()
	value.OCRShortcut = &ocr
	value.TranslateShortcut = &translate
	selection := s.SelectionBinding()
	value.SelectionShortcut = &selection
	quick := s.QuickAssistantBinding()
	value.QuickAssistantShortcut = &quick
	open := s.OpenBinding()
	value.OpenShortcut = &open
	return value
}

// Store serializes writes and exposes immutable value snapshots.
type Store struct {
	mu    sync.Mutex
	path  string
	value Settings
}

// Open loads an existing file or uses defaults only when the file is absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, value: Settings{SchemaVersion: 1, Theme: "system", CaptureShortcut: "Shift+A", PinShortcut: "Alt+Q"}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &s.value); err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	if err = validate(s.value); err != nil {
		return nil, err
	}
	return s, nil
}

func validate(s Settings) error {
	switch s.DefaultPage {
	case "", "overview", "capture", "selection", "quick-assistant", "wintray", "convenience", "chat", "ai-services", "settings":
	default:
		return errors.New("unsupported default page")
	}
	if s.Language != "" && s.Language != "system" && s.Language != "zh-CN" && s.Language != "en-US" {
		return errors.New("unsupported language")
	}
	if s.SchemaVersion != 1 {
		return errors.New("unsupported settings version")
	}
	if s.Theme != "system" && s.Theme != "light" && s.Theme != "dark" {
		return errors.New("invalid theme")
	}
	capture, err := hotkeys.Parse(s.CaptureShortcut)
	if err != nil {
		return err
	}
	pin, err := hotkeys.Parse(s.PinShortcut)
	if err != nil {
		return err
	}
	if capture.Key != 0 && capture == pin {
		return errors.New("截图和置顶不能使用相同的快捷键")
	}
	return hotkeys.ValidateBindings([]hotkeys.Binding{{ID: "capture", Shortcut: s.CaptureShortcut}, {ID: "ocr", Shortcut: s.OCRBinding()}, {ID: "translate", Shortcut: s.TranslateBinding()}, {ID: "pin", Shortcut: s.PinShortcut}, {ID: "selection", Shortcut: s.SelectionBinding()}, {ID: "quick-assistant", Shortcut: s.QuickAssistantBinding()}, {ID: "open-winone", Shortcut: s.OpenBinding()}})
}

// Validate checks global preferences without opening or writing a legacy store.
func Validate(s Settings) error { return validate(s) }

// Snapshot returns a copy which the caller can submit with its revision.
func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copySettings(s.value)
}

// Save only publishes the new in-memory state after a durable file replacement.
func (s *Store) Save(next Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next.Revision != s.value.Revision {
		return copySettings(s.value), ErrConflict
	}
	if err := validate(next); err != nil {
		return copySettings(s.value), err
	}
	next.Revision++
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return copySettings(s.value), err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return copySettings(s.value), err
	}
	// The fixed temporary name is safe because the Host is the sole writer.
	tmp := s.path + ".pending"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return copySettings(s.value), err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return copySettings(s.value), err
	}
	if old, readErr := os.ReadFile(s.path); readErr == nil {
		if err = os.WriteFile(s.path+".backup", old, 0600); err != nil {
			return copySettings(s.value), err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return copySettings(s.value), readErr
	}
	if err = replaceFile(tmp, s.path); err != nil {
		return copySettings(s.value), err
	}
	s.value = copySettings(next)
	return copySettings(next), nil
}

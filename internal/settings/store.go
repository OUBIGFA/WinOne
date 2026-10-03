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
	SchemaVersion   int                 `json:"schemaVersion"`
	Revision        uint64              `json:"revision"`
	Theme           string              `json:"theme"`
	Language        string              `json:"language,omitempty"`
	CaptureShortcut string              `json:"captureShortcut"`
	PinShortcut     string              `json:"pinShortcut"`
	Startup         *StartupPreferences `json:"startup,omitempty"`
}

// Startup belongs to WinOne, not to a module. Nil is a legacy document which
// the configuration store migrates from the old WinTray compatibility fields.
type StartupPreferences struct {
	RunAtLogon          bool `json:"runAtLogon"`
	StartMinimized      bool `json:"startMinimized"`
	PreferAdministrator bool `json:"preferAdministrator,omitempty"`
}

func copySettings(value Settings) Settings {
	if value.Startup != nil {
		startup := *value.Startup
		value.Startup = &startup
	}
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
	return nil
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

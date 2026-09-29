// Package settings owns global preferences and rejects stale client updates.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrConflict = errors.New("settings changed; reload before saving")

// Settings contains only global preferences; feature settings belong to their modules.
type Settings struct {
	SchemaVersion int    `json:"schemaVersion"`
	Revision      uint64 `json:"revision"`
	Theme         string `json:"theme"`
}

// Store serializes writes and exposes immutable value snapshots.
type Store struct {
	mu    sync.Mutex
	path  string
	value Settings
}

// Open loads an existing file or uses defaults only when the file is absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, value: Settings{SchemaVersion: 1, Theme: "system"}}
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
	if s.SchemaVersion != 1 {
		return errors.New("unsupported settings version")
	}
	if s.Theme != "system" && s.Theme != "light" && s.Theme != "dark" {
		return errors.New("invalid theme")
	}
	return nil
}

// Snapshot returns a copy which the caller can submit with its revision.
func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

// Save only publishes the new in-memory state after a durable file replacement.
func (s *Store) Save(next Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next.Revision != s.value.Revision {
		return s.value, ErrConflict
	}
	if err := validate(next); err != nil {
		return s.value, err
	}
	next.Revision++
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return s.value, err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return s.value, err
	}
	// The fixed temporary name is safe because the Host is the sole writer.
	tmp := s.path + ".pending"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return s.value, err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return s.value, err
	}
	if old, readErr := os.ReadFile(s.path); readErr == nil {
		if err = os.WriteFile(s.path+".backup", old, 0600); err != nil {
			return s.value, err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return s.value, readErr
	}
	if err = replaceFile(tmp, s.path); err != nil {
		return s.value, err
	}
	s.value = next
	return next, nil
}

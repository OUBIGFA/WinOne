package settings

import (
	"errors"
	"os"
	"path/filepath"
)

// WriteAtomic persists a complete settings document before replacing its predecessor.
// Callers serialize writes and own schema/revision validation.
func WriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".winone-settings-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

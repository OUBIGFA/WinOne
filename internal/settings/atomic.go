package settings

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// WriteAtomic persists a complete settings document before replacing its predecessor.
// Callers serialize writes and own schema/revision validation.
func WriteAtomic(path string, data []byte) error {
	return WriteAtomicStream(path, func(w io.Writer) error { _, err := io.Copy(w, bytes.NewReader(data)); return err })
}

// WriteAtomicStream keeps large exports out of memory. A failed producer or
// ownership check leaves the previous destination intact.
func WriteAtomicStream(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".winone-settings-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	writeErr := write(file)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

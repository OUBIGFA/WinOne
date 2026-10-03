package distribution

import (
	"errors"
	"os"
)

// The updater holds its exclusive maintenance lease while adding snapshots.
// Publish only after a complete write; never replace a completed snapshot.
func writeBackupMetadata(directory, name string, data []byte) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return errors.New("metadata snapshot already exists or cannot be checked")
	}
	temporary := "." + name + ".tmp"
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	return root.Rename(temporary, name)
}

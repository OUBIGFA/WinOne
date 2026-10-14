package settings

import (
	"bytes"
	"errors"
	"os"
)

// PreserveMigrationBackup retains the pre-migration journal, including pending
// recovery state. Repeating a failed migration may reuse only an identical copy.
func PreserveMigrationBackup(path string, original []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if !bytes.Equal(existing, original) {
			return errors.New("配置迁移备份已有不同内容，请保留备份后重试")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return WriteAtomic(path, original)
}

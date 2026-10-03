package preferences

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"winone/modules/wintray/internal/config"
)

// MergeHistory imports launch facts without starting a worker or applying any
// system setting. The shared file lock also covers worker reservations/refunds.
func MergeHistory(directory string, data json.RawMessage) error {
	if directory == "" {
		return errors.New("运行历史目录不可用")
	}
	return config.NewStartupHistory(filepath.Join(directory, "startup-history.json")).MergeImport(data)
}

package configuration

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"winone/internal/configjson"
	"winone/internal/settings"
)

// ReadCommittedSettings is the read-only bootstrap path. It does not recover
// transactions, apply module settings, or use an uncommitted startup preference.
func ReadCommittedSettings(directory string) (settings.Settings, error) {
	var result settings.Settings
	file, err := os.Open(filepath.Join(directory, "configuration.json"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil {
		return result, err
	}
	if len(data) > 8<<20 {
		return result, errors.New("配置文件过大")
	}
	var state diskState
	if err = configjson.DecodeObject(data, &state); err != nil {
		return result, err
	}
	if state.Committed.SchemaVersion != 1 {
		return result, errors.New("配置版本不受支持")
	}
	if err = configjson.DecodeObject(state.Committed.Values["host"], &result); err != nil {
		return result, err
	}
	if err = settings.Validate(result); err != nil {
		return result, err
	}
	result.Revision = state.Committed.Revision
	return result, nil
}

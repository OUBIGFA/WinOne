//go:build windows

package onboarding

import (
	"errors"
	"os"
	"path/filepath"
)

const ResultSendHintMarker = "result-send-hint-v1.once"

// CREATE_NEW is the machine-wide winner election, including simultaneous Hosts
// using different profiles. It never reads or writes any conversation content.
func ClaimSendHint(directory string) (bool, error) {
	release, err := LockDirectory(directory)
	if err != nil {
		return false, err
	}
	defer release()
	path := filepath.Join(directory, ResultSendHintMarker)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0660)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, writeErr := file.WriteString("1\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return false, errors.Join(err, os.Remove(path))
	}
	return true, nil
}

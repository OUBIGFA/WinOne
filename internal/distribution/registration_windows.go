//go:build windows

package distribution

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows/registry"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// BackupRegistration adds a registry snapshot to an already verified backup.
// Callers hold the installation's exclusive maintenance lease through backup
// and upgrade. Existing snapshots are never intentionally overwritten.
func BackupRegistration(directory, referencePackage, backupDirectory, keyPath string) error {
	receipt, err := VerifyBackup(backupDirectory, referencePackage)
	if err != nil {
		return err
	}
	if _, err = VerifyInstalled(directory, referencePackage); err != nil {
		return err
	}
	snapshot, err := CaptureRegistration(directory, keyPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		SchemaVersion  int                  `json:"schemaVersion"`
		ManifestSHA256 string               `json:"manifestSha256"`
		Registration   RegistrationSnapshot `json:"registration"`
	}{1, receipt.ManifestSHA256, snapshot}, "", "  ")
	if err != nil {
		return err
	}
	return writeBackupMetadata(backupDirectory, "registration.json", data)
}

type RegistrationValue struct {
	Name string `json:"name"`
	Type uint32 `json:"type"`
	Data []byte `json:"data"`
}

type RegistrationSnapshot struct {
	SchemaVersion int                 `json:"schemaVersion"`
	KeyPath       string              `json:"keyPath"`
	Exists        bool                `json:"exists"`
	Values        []RegistrationValue `json:"values"`
}

// CaptureRegistration reads only the selected current-user, 64-bit uninstall
// key. Existing registration must belong to the supplied installation. Raw
// bytes preserve value types without expanding environment strings. Callers
// retain their maintenance lease; concurrent registry changes are rejected.
func CaptureRegistration(directory, keyPath string) (RegistrationSnapshot, error) {
	return captureRegistration(directory, keyPath, true)
}

func captureRegistration(directory, keyPath string, checkOwner bool) (RegistrationSnapshot, error) {
	result := RegistrationSnapshot{SchemaVersion: 1, KeyPath: keyPath, Values: []RegistrationValue{}}
	if keyPath == "" || strings.HasPrefix(keyPath, `\`) {
		return result, errors.New("invalid registration key")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.READ|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer key.Close()
	before, err := key.Stat()
	if err != nil {
		return result, err
	}
	if before.SubKeyCount != 0 || before.ValueCount > 64 {
		return result, errors.New("registration exceeds flat snapshot limits")
	}
	if checkOwner {
		location, _, err := key.GetStringValue("InstallLocation")
		if err != nil {
			return result, err
		}
		if !filepath.IsAbs(location) {
			return result, errors.New("registration location is not absolute")
		}
		expected, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return result, err
		}
		expected, err = filepath.Abs(expected)
		if err != nil {
			return result, err
		}
		actual, err := filepath.EvalSymlinks(location)
		if err != nil {
			return result, err
		}
		if !strings.EqualFold(filepath.Clean(expected), filepath.Clean(actual)) {
			return result, errors.New("registration belongs to another installation")
		}
	}
	names, err := key.ReadValueNames(65)
	if err != nil && !errors.Is(err, io.EOF) {
		return result, err
	}
	if len(names) > 64 {
		return result, errors.New("too many registration values")
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		size, kind, err := key.GetValue(name, nil)
		if err != nil {
			return result, err
		}
		if size < 0 || size > 65536 {
			return result, errors.New("registration value exceeds size limit")
		}
		total += size
		if total > 1<<20 {
			return result, errors.New("registration snapshot exceeds size limit")
		}
		data := make([]byte, size)
		n, readKind, err := key.GetValue(name, data)
		if err != nil {
			return result, err
		}
		if n != size || readKind != kind {
			return result, errors.New("registration changed during snapshot")
		}
		result.Values = append(result.Values, RegistrationValue{Name: name, Type: kind, Data: data})
	}
	after, err := key.Stat()
	if err != nil {
		return result, err
	}
	if before.ValueCount != after.ValueCount || !before.ModTime().Equal(after.ModTime()) {
		return result, errors.New("registration changed during snapshot")
	}
	result.Exists = true
	return result, nil
}

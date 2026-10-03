package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// ApplyUpgradeUninstaller applies a caller-verified generated uninstaller. Nil
// descriptors explicitly represent absence, e.g. a portable installation.
func ApplyUpgradeUninstaller(installation, staged string, original, next *File, journal *UpgradeJournalStore) error {
	return executeUpgradeUninstaller(installation, staged, original, next, journal, false)
}

// RestoreUpgradeUninstaller restores the old generated file under journal control.
func RestoreUpgradeUninstaller(installation, backup string, original, next *File, journal *UpgradeJournalStore) error {
	return executeUpgradeUninstaller(installation, backup, original, next, journal, true)
}

func executeUpgradeUninstaller(installation, sourceDirectory string, original, next *File, journal *UpgradeJournalStore, restore bool) error {
	for _, file := range []*File{original, next} {
		if err := validateUninstallerDescriptor(file); err != nil {
			return err
		}
	}
	root, err := openUpgradeRoot(installation)
	if err != nil {
		return err
	}
	defer root.Close()
	if marker, err := root.Lstat(".winone-pending"); err != nil || !marker.Mode().IsRegular() {
		return errors.New("uninstaller update requires pending marker")
	}
	match := func(file *File, hash string, size int64) bool {
		if file == nil {
			return hash == ""
		}
		return hash == file.SHA256 && size == file.Size
	}
	var currentHash string
	inspect := func() (metadataObservation, error) {
		info, err := root.Lstat("Uninstall.exe")
		var size int64
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return metadataObservation{}, err
		}
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() > 16<<20 {
				return metadataObservation{}, errors.New("invalid current uninstaller")
			}
			size = info.Size()
		}
		currentHash, err = upgradeFileHash(root, "Uninstall.exe")
		if err != nil {
			return metadataObservation{}, err
		}
		return metadataObservation{before: match(original, currentHash, size), after: match(next, currentHash, size)}, nil
	}
	desired := next
	if restore {
		desired = original
	}
	write := func() error {
		if desired == nil {
			return root.Remove("Uninstall.exe")
		}
		source, err := openUpgradeRoot(sourceDirectory)
		if err != nil {
			return err
		}
		defer source.Close()
		info, err := source.Lstat("Uninstall.exe")
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != desired.Size {
			return errors.New("source uninstaller size differs")
		}
		return replaceUpgradeFile(root, source, "Uninstall.exe", "Uninstall.exe", desired.SHA256, currentHash)
	}
	equal := (original == nil && next == nil) || (original != nil && next != nil && *original == *next)
	return runMetadataOperation(journal, "uninstaller", uninstallerOperationFingerprint(original, next), equal, restore, inspect, write)
}

func validateUninstallerDescriptor(file *File) error {
	if file == nil {
		return nil
	}
	digest, err := hex.DecodeString(file.SHA256)
	if err != nil || len(digest) != 32 || file.SHA256 != strings.ToLower(file.SHA256) || file.Path != "Uninstall.exe" || file.Size < 0 || file.Size > 16<<20 {
		return errors.New("invalid uninstaller descriptor")
	}
	return nil
}

func uninstallerOperationFingerprint(original, next *File) string {
	data, _ := json.Marshal([]*File{original, next})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

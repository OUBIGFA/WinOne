package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/configjson"
)

// VerifyInstalled compares only the package-owned files and manifest. Unknown
// files are not opened or changed. This is a read-only observation, not a lock
// or a guarantee that files will remain unchanged before an upgrade starts.
func VerifyInstalled(directory, referencePackage string) (Manifest, error) {
	return verifyInstalled(directory, referencePackage, false)
}

// VerifyUpgradePayload checks bytes while the pending marker deliberately keeps
// normal startup blocked. It does not certify a committed or runnable upgrade.
func VerifyUpgradePayload(directory, referencePackage string) (Manifest, error) {
	return verifyInstalled(directory, referencePackage, true)
}

func verifyInstalled(directory, referencePackage string, pending bool) (Manifest, error) {
	// This compares bytes to their own release inventory, including backups
	// of older dependencies. Candidate admission is the caller's Verify step.
	expected, err := verifyHistoricalPackage(referencePackage)
	if err != nil {
		return Manifest{}, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return Manifest{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, errors.New("installation root must be a real directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	marker, markerErr := root.Lstat(".winone-pending")
	if pending {
		if markerErr != nil || !marker.Mode().IsRegular() {
			return Manifest{}, errors.New("upgrade verification requires a pending operation")
		}
	} else if markerErr == nil || !errors.Is(markerErr, os.ErrNotExist) {
		return Manifest{}, errors.New("installation has an incomplete operation or cannot be inspected")
	}
	installed, _, err := readInstalledManifest(root)
	if err != nil {
		return Manifest{}, err
	}
	if manifestFingerprint(installed) != manifestFingerprint(expected) {
		return Manifest{}, errors.New("installed manifest does not match reference package")
	}
	for _, record := range expected.Files {
		file, err := openInstalledFile(root, record.Path)
		if err != nil {
			return Manifest{}, fmt.Errorf("inspect %s: %w", record.Path, err)
		}
		digest := sha256.New()
		size, readErr := io.Copy(digest, io.LimitReader(file, record.Size+1))
		closeErr := file.Close()
		if err = errors.Join(readErr, closeErr); err != nil {
			return Manifest{}, fmt.Errorf("read %s: %w", record.Path, err)
		}
		if size != record.Size || hex.EncodeToString(digest.Sum(nil)) != record.SHA256 {
			return Manifest{}, fmt.Errorf("installed file differs: %s", record.Path)
		}
	}
	return expected, nil
}

func readInstalledManifest(root *os.Root) (Manifest, []byte, error) {
	var result Manifest
	file, err := openInstalledFile(root, "manifest.json")
	if err != nil {
		return result, nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return result, nil, err
	}
	if len(data) > 1<<20 {
		return result, nil, errors.New("installed manifest exceeds size limit")
	}
	if err = configjson.DecodeObject(data, &result); err != nil {
		return result, nil, err
	}
	return result, data, nil
}

func openInstalledFile(root *os.Root, name string) (*os.File, error) {
	// Reject links in every ancestor as well as the leaf. Root.Open also
	// confines resolution if a path changes between checking and opening.
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("installation contains a link")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("installation parent is not a directory")
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, errors.New("installation entry is not a regular file")
		}
	}
	return root.Open(filepath.FromSlash(name))
}

package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type BackupRecord struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Version                string `json:"version"`
	ManifestSHA256         string `json:"manifestSha256"`
	FileCount              int    `json:"fileCount"`
	InstallerFilesCaptured bool   `json:"installerFilesCaptured"`
	InstallerFiles         []File `json:"installerFiles,omitempty"`
}

// BackupInstalled copies verified package-owned bytes and the optional generated
// uninstaller, never configuration or arbitrary extra files. The caller must
// quiesce the application and retain its maintenance
// lease for subsequent changes. Partial output is retained without a completion
// record, and an existing backup is never overwritten.
func BackupInstalled(directory, referencePackage, destination string) (BackupRecord, error) {
	manifest, err := VerifyInstalled(directory, referencePackage)
	if err != nil {
		return BackupRecord{}, err
	}
	sourcePath, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return BackupRecord{}, err
	}
	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return BackupRecord{}, err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return BackupRecord{}, err
	}
	parentPath, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return BackupRecord{}, err
	}
	resolved := filepath.Join(parentPath, filepath.Base(destination))
	relative, relErr := filepath.Rel(sourcePath, resolved)
	if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return BackupRecord{}, errors.New("backup must be outside installation")
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return BackupRecord{}, err
	}
	defer parent.Close()
	name := filepath.Base(destination)
	if err = parent.Mkdir(name, 0700); err != nil {
		return BackupRecord{}, err
	}
	target, err := parent.OpenRoot(name)
	if err != nil {
		return BackupRecord{}, err
	}
	defer target.Close()
	source, err := os.OpenRoot(directory)
	if err != nil {
		return BackupRecord{}, err
	}
	defer source.Close()
	for _, record := range manifest.Files {
		input, err := openInstalledFile(source, record.Path)
		if err != nil {
			return BackupRecord{}, err
		}
		path := filepath.FromSlash(record.Path)
		if err = target.MkdirAll(filepath.Dir(path), 0700); err != nil {
			input.Close()
			return BackupRecord{}, err
		}
		output, err := target.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return BackupRecord{}, err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, record.Size+1))
		err = errors.Join(copyErr, input.Close(), output.Sync(), output.Close())
		if err != nil {
			return BackupRecord{}, err
		}
		if size != record.Size || hex.EncodeToString(digest.Sum(nil)) != record.SHA256 {
			return BackupRecord{}, fmt.Errorf("source changed during backup: %s", record.Path)
		}
	}
	// Normalize the already validated manifest so no second untrusted manifest
	// read can substitute the inventory after source verification.
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupRecord{}, err
	}
	write := func(name string, data []byte) error {
		file, err := target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		return errors.Join(writeErr, file.Sync(), file.Close())
	}
	if err = write("manifest.json", data); err != nil {
		return BackupRecord{}, err
	}
	if _, err = VerifyInstalled(resolved, referencePackage); err != nil {
		return BackupRecord{}, err
	}
	record := BackupRecord{SchemaVersion: 1, Version: manifest.Version, ManifestSHA256: manifestFingerprint(manifest), FileCount: len(manifest.Files)}
	// NSIS generates this file during installation, so it is not in the ZIP
	// manifest. Record absence explicitly; older payload-only backups have not
	// captured installer files and must not be treated as complete installer state.
	info, statErr := source.Lstat("Uninstall.exe")
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return BackupRecord{}, statErr
	}
	if statErr == nil {
		const limit = 16 << 20
		if !info.Mode().IsRegular() || info.Size() > limit {
			return BackupRecord{}, errors.New("invalid installed uninstaller")
		}
		file, err := openInstalledFile(source, "Uninstall.exe")
		if err != nil {
			return BackupRecord{}, err
		}
		content, readErr := io.ReadAll(io.LimitReader(file, limit+1))
		if err = errors.Join(readErr, file.Close()); err != nil {
			return BackupRecord{}, err
		}
		if len(content) > limit || int64(len(content)) != info.Size() {
			return BackupRecord{}, errors.New("uninstaller changed during backup")
		}
		if err = write("Uninstall.exe", content); err != nil {
			return BackupRecord{}, err
		}
		digest := sha256.Sum256(content)
		check, err := target.Open("Uninstall.exe")
		if err != nil {
			return BackupRecord{}, err
		}
		copied := sha256.New()
		size, readErr := io.Copy(copied, io.LimitReader(check, int64(len(content))+1))
		if err = errors.Join(readErr, check.Close()); err != nil {
			return BackupRecord{}, err
		}
		checksum := hex.EncodeToString(digest[:])
		if size != int64(len(content)) || hex.EncodeToString(copied.Sum(nil)) != checksum {
			return BackupRecord{}, errors.New("uninstaller backup verification failed")
		}
		record.InstallerFiles = []File{{Path: "Uninstall.exe", Size: size, SHA256: checksum}}
	}
	record.InstallerFilesCaptured = true
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		return BackupRecord{}, err
	}
	if err = write(".backup-complete.tmp", data); err != nil {
		return BackupRecord{}, err
	}
	if err = target.Rename(".backup-complete.tmp", "backup-complete.json"); err != nil {
		return BackupRecord{}, err
	}
	return record, nil
}

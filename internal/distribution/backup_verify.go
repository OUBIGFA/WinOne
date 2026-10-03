package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"winone/internal/configjson"
)

// VerifyBackup checks both the verified payload and installer-owned file
// snapshot. A completion filename alone never proves a usable backup.
func VerifyBackup(directory, referencePackage string) (BackupRecord, error) {
	manifest, err := VerifyInstalled(directory, referencePackage)
	if err != nil {
		return BackupRecord{}, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return BackupRecord{}, err
	}
	defer root.Close()
	file, err := openInstalledFile(root, "backup-complete.json")
	if err != nil {
		return BackupRecord{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 65537))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return BackupRecord{}, err
	}
	if len(data) > 65536 {
		return BackupRecord{}, errors.New("backup record too large")
	}
	var record BackupRecord
	if err = configjson.DecodeObject(data, &record); err != nil {
		return BackupRecord{}, err
	}
	if record.SchemaVersion != 1 || record.Version != manifest.Version || record.ManifestSHA256 != manifestFingerprint(manifest) || record.FileCount != len(manifest.Files) || !record.InstallerFilesCaptured || len(record.InstallerFiles) > 1 {
		return BackupRecord{}, errors.New("backup record does not match its payload")
	}
	if len(record.InstallerFiles) == 0 {
		if _, err := root.Lstat("Uninstall.exe"); !errors.Is(err, os.ErrNotExist) {
			return BackupRecord{}, errors.New("unexpected or unreadable uninstaller in backup")
		}
		return record, nil
	}
	entry := record.InstallerFiles[0]
	if entry.Path != "Uninstall.exe" || entry.Size < 0 || entry.Size > 16<<20 || len(entry.SHA256) != 64 {
		return BackupRecord{}, errors.New("invalid installer file record")
	}
	file, err = openInstalledFile(root, entry.Path)
	if err != nil {
		return BackupRecord{}, err
	}
	digest := sha256.New()
	size, readErr := io.Copy(digest, io.LimitReader(file, entry.Size+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return BackupRecord{}, err
	}
	if size != entry.Size || hex.EncodeToString(digest.Sum(nil)) != entry.SHA256 {
		return BackupRecord{}, errors.New("uninstaller backup checksum mismatch")
	}
	return record, nil
}

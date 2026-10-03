package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxShortcutBytes = 1 << 20

var shellLinkHeader = []byte{0x4c, 0, 0, 0, 1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}

// BackupShortcut binds an exact shortcut snapshot to the verified old payload.
// It does not modify the shortcut or attempt to launch its target.
func BackupShortcut(referencePackage, backupDirectory, shortcutPath string) error {
	receipt, err := VerifyBackup(backupDirectory, referencePackage)
	if err != nil {
		return err
	}
	snapshot, err := CaptureShortcut(shortcutPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		SchemaVersion  int              `json:"schemaVersion"`
		ManifestSHA256 string           `json:"manifestSha256"`
		Shortcut       ShortcutSnapshot `json:"shortcut"`
	}{1, receipt.ManifestSHA256, snapshot}, "", "  ")
	if err != nil {
		return err
	}
	return writeBackupMetadata(backupDirectory, "shortcut.json", data)
}

type ShortcutSnapshot struct {
	SchemaVersion int    `json:"schemaVersion"`
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	SHA256        string `json:"sha256,omitempty"`
	Data          []byte `json:"data,omitempty"`
}

// CaptureShortcut reads bytes without loading or resolving a Shell link, so
// network paths and executable targets are never followed. The format header
// check identifies the file type; it does not certify the link's destination.
func CaptureShortcut(path string) (ShortcutSnapshot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ShortcutSnapshot{}, err
	}
	result := ShortcutSnapshot{SchemaVersion: 1, Path: absolute}
	if !strings.EqualFold(filepath.Ext(path), ".lnk") {
		return result, errors.New("shortcut must have .lnk extension")
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxShortcutBytes {
		return result, errors.New("invalid shortcut file")
	}
	parent, err := os.Lstat(filepath.Dir(absolute))
	if err != nil {
		return result, err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("shortcut parent must be a real directory")
	}
	root, err := os.OpenRoot(filepath.Dir(absolute))
	if err != nil {
		return result, err
	}
	defer root.Close()
	file, err := openInstalledFile(root, filepath.Base(absolute))
	if err != nil {
		return result, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return result, err
	}
	if !os.SameFile(info, opened) {
		file.Close()
		return result, errors.New("shortcut changed while opening")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxShortcutBytes+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return result, err
	}
	if int64(len(data)) != info.Size() {
		return result, errors.New("shortcut changed while reading")
	}
	sum := sha256.Sum256(data)
	result.Exists, result.Data, result.SHA256 = true, data, hex.EncodeToString(sum[:])
	return result, ValidateShortcutSnapshot(result)
}

// ValidateShortcutSnapshot checks a stored snapshot before any future restore.
// Restoration must additionally verify the caller's intended destination.
func ValidateShortcutSnapshot(snapshot ShortcutSnapshot) error {
	if snapshot.SchemaVersion != 1 || !filepath.IsAbs(snapshot.Path) || !strings.EqualFold(filepath.Ext(snapshot.Path), ".lnk") {
		return errors.New("invalid shortcut snapshot")
	}
	if !snapshot.Exists {
		if len(snapshot.Data) != 0 || snapshot.SHA256 != "" {
			return errors.New("absent shortcut has unexpected contents")
		}
		return nil
	}
	if len(snapshot.Data) < 76 || len(snapshot.Data) > maxShortcutBytes || !bytes.HasPrefix(snapshot.Data, shellLinkHeader) {
		return errors.New("invalid Shell link header or size")
	}
	sum := sha256.Sum256(snapshot.Data)
	if hex.EncodeToString(sum[:]) != snapshot.SHA256 {
		return errors.New("shortcut snapshot checksum mismatch")
	}
	return nil
}

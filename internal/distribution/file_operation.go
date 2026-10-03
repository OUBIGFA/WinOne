package distribution

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// These per-file operations require serialized caller scheduling and an exclusive maintenance lease,
// validated staging/backup and package plan. They leave the pending marker in
// place: metadata restoration, whole-payload validation and startup confirmation
// belong to the transaction coordinator, not to one successful file write.
func ApplyUpgradeFile(installation, staged string, journal *UpgradeJournalStore, index int) error {
	return executeUpgradeFile(installation, staged, journal, index, false)
}
func RestoreUpgradeFile(installation, backup string, journal *UpgradeJournalStore, index int) error {
	return executeUpgradeFile(installation, backup, journal, index, true)
}

func executeUpgradeFile(installation, sourceDirectory string, journal *UpgradeJournalStore, index int, restore bool) error {
	state := journal.Snapshot()
	if index < 0 || index >= len(state.Plan.Files) {
		return errors.New("invalid upgrade file index")
	}
	if (!restore && state.Phase != "applying") || (restore && state.Phase != "restoring") {
		return errors.New("upgrade phase does not allow file operation")
	}
	file, status := state.Plan.Files[index], state.FileStates[index]
	root, err := openUpgradeRoot(installation)
	if err != nil {
		return err
	}
	defer root.Close()
	if info, err := root.Lstat(".winone-pending"); err != nil || !info.Mode().IsRegular() {
		return errors.New("upgrade requires an incomplete-operation marker")
	}
	current, err := upgradeFileHash(root, file.Path)
	if err != nil {
		return err
	}
	if status == "unchanged" {
		if current != file.PreviousSHA256 {
			return errors.New("unchanged file was modified")
		}
		return nil
	}
	desired, prior, sourceName, intent, complete := file.NextSHA256, file.PreviousSHA256, file.Path, "writing", "applied"
	if restore {
		desired, prior, sourceName, intent, complete = file.PreviousSHA256, file.NextSHA256, file.PreviousPath, "restoring", "restored"
	}
	if status == complete {
		if current != desired {
			return errors.New("completed file changed externally")
		}
		return nil
	}
	if !restore && status != "pending" && status != "writing" {
		return errors.New("invalid apply file state")
	}
	if restore && status != "writing" && status != "applied" && status != "restoring" {
		return errors.New("invalid restore file state")
	}
	// Only recorded in-flight work may already contain the desired bytes.
	if current != prior && !(status != "pending" && current == desired) {
		return errors.New("file differs from both expected upgrade states")
	}
	if status != intent {
		if err = journal.RecordFile(state.Revision, index, intent); err != nil {
			return err
		}
	}
	if current != desired {
		if desired == "" {
			if err = root.Remove(filepath.FromSlash(file.Path)); err != nil {
				return err
			}
		} else {
			source, err := openUpgradeRoot(sourceDirectory)
			if err != nil {
				return err
			}
			err = replaceUpgradeFile(root, source, file.Path, sourceName, desired, current)
			source.Close()
			if err != nil {
				return err
			}
		}
	}
	actual, err := upgradeFileHash(root, file.Path)
	if err != nil {
		return err
	}
	if actual != desired {
		return errors.New("upgrade file verification failed")
	}
	return journal.RecordFile(journal.Snapshot().Revision, index, complete)
}

func openUpgradeRoot(directory string) (*os.Root, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("upgrade directory must not be a link")
	}
	return os.OpenRoot(directory)
}

func upgradeFileHash(root *os.Root, name string) (string, error) {
	file, err := openInstalledFile(root, name)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	size, readErr := io.Copy(digest, io.LimitReader(file, maxFileBytes+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return "", err
	}
	if size > maxFileBytes {
		return "", errors.New("upgrade file exceeds size limit")
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func replaceUpgradeFile(target, source *os.Root, destination, sourceName, want, current string) error {
	input, err := openInstalledFile(source, sourceName)
	if err != nil {
		return err
	}
	defer input.Close()
	parts := strings.Split(destination, "/")
	for i := 1; i < len(parts); i++ {
		parent := filepath.Join(parts[:i]...)
		info, err := target.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			if err = target.Mkdir(parent, 0700); err != nil {
				return err
			}
			info, err = target.Lstat(parent)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("upgrade parent is not a real directory")
		}
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(filepath.FromSlash(destination)), ".winone-upgrade-"+hex.EncodeToString(random[:])+".tmp")
	output, err := target.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer target.Remove(temporary)
	digest := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, maxFileBytes+1))
	if err = errors.Join(copyErr, output.Sync(), output.Close()); err != nil {
		return err
	}
	if size > maxFileBytes || hex.EncodeToString(digest.Sum(nil)) != want {
		return errors.New("staged or backup file checksum mismatch")
	}
	actual, err := upgradeFileHash(target, destination)
	if err != nil {
		return err
	}
	if actual != current {
		return errors.New("target changed while preparing replacement")
	}
	return target.Rename(temporary, filepath.FromSlash(destination))
}

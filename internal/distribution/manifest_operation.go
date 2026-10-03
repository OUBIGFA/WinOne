package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ApplyUpgradeManifest switches the inventory after payload work is recorded.
// The coordinator supplies verified sources and holds the maintenance lease.
func ApplyUpgradeManifest(installation, staged string, journal *UpgradeJournalStore) error {
	return executeUpgradeManifest(installation, staged, journal, false)
}
// RestoreUpgradeManifest restores the inventory only after payload recovery.
func RestoreUpgradeManifest(installation, backup string, journal *UpgradeJournalStore) error {
	return executeUpgradeManifest(installation, backup, journal, true)
}

// Manifest replacement follows payload replacement/restoration. The marker
// remains until the coordinator has handled all metadata and startup checks.
func executeUpgradeManifest(installation, sourceDirectory string, journal *UpgradeJournalStore, restore bool) error {
	state := journal.Snapshot()
	if (!restore && state.Phase != "applying") || (restore && state.Phase != "restoring") {
		return errors.New("invalid manifest operation phase")
	}
	for _, file := range state.FileStates {
		ready := file == "unchanged" || (!restore && file == "applied") || (restore && (file == "restored" || file == "pending"))
		if !ready {
			return errors.New("payload files must finish before manifest")
		}
	}
	want, prior, intent, complete := state.Plan.NextManifestSHA256, state.Plan.PreviousManifestSHA256, "writing", "applied"
	if restore {
		want, prior, intent, complete = state.Plan.PreviousManifestSHA256, state.Plan.NextManifestSHA256, "restoring", "restored"
	}
	root, err := openUpgradeRoot(installation)
	if err != nil {
		return err
	}
	defer root.Close()
	if marker, err := root.Lstat(".winone-pending"); err != nil || !marker.Mode().IsRegular() {
		return errors.New("manifest operation requires pending marker")
	}
	current, raw, err := readInstalledManifest(root)
	if err != nil {
		return err
	}
	currentHash := manifestFingerprint(current)
	status := state.MetadataStates["manifest"]
	if status == complete || (restore && status == "pending") {
		if currentHash != want {
			return errors.New("manifest changed outside recorded work")
		}
		return nil
	}
	if (!restore && status != "pending" && status != "writing") || (restore && status != "writing" && status != "applied" && status != "restoring") {
		return errors.New("invalid manifest progress")
	}
	if currentHash != prior && !(status != "pending" && currentHash == want) {
		return errors.New("manifest differs from expected versions")
	}
	if status != intent {
		if err = journal.RecordMetadata(state.Revision, "manifest", intent); err != nil {
			return err
		}
	}
	if currentHash != want {
		source, err := openUpgradeRoot(sourceDirectory)
		if err != nil {
			return err
		}
		defer source.Close()
		manifest, data, err := readInstalledManifest(source)
		if err != nil {
			return err
		}
		if manifestFingerprint(manifest) != want {
			return errors.New("source manifest does not match upgrade plan")
		}
		oldBytes, newBytes := sha256.Sum256(raw), sha256.Sum256(data)
		if err = replaceUpgradeFile(root, source, "manifest.json", "manifest.json", hex.EncodeToString(newBytes[:]), hex.EncodeToString(oldBytes[:])); err != nil {
			return err
		}
	}
	actual, _, err := readInstalledManifest(root)
	if err != nil {
		return err
	}
	if manifestFingerprint(actual) != want {
		return errors.New("manifest replacement verification failed")
	}
	return journal.RecordMetadata(journal.Snapshot().Revision, "manifest", complete)
}

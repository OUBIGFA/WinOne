//go:build windows

package distribution

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"winone/internal/configjson"
	"winone/internal/maintenance"
)

type UpgradeExecution struct {
	Installation, Profile, Staged, Backup string
	PreviousPackage, NextPackage          string
	RegistryKey, ShortcutPath             string
	NextRegistration                      RegistrationSnapshot
	NextShortcut                          ShortcutSnapshot
	NextUninstaller                       *File
	Journal                               *UpgradeJournalStore
	// Prepare quiesces workers/restores resources under the acquired lease.
	// Nil uses PrepareNativeUpgrade; tests may inject a controlled implementation.
	Prepare func() error
	// CheckStartup must perform a bounded, isolated check without migrating
	// live user configuration. Nil runs all native target checks.
	CheckStartup func() error
}

// ExecuteUpgrade coordinates verified payload and metadata changes. The caller
// supplies trusted target metadata and real lifecycle/probe implementations;
// this does not authenticate a remote release or implement an online updater.
func ExecuteUpgrade(r UpgradeExecution) error {
	if r.Journal == nil {
		return errors.New("upgrade journal is required")
	}
	live, err := OpenUpgradeJournal(r.Journal.path)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(live.Snapshot(), r.Journal.Snapshot()) {
		return errors.New("upgrade journal is stale")
	}
	// Freeze caller-owned slices before callbacks or file operations run.
	r.NextShortcut.Data = append([]byte(nil), r.NextShortcut.Data...)
	r.NextRegistration.Values = append([]RegistrationValue(nil), r.NextRegistration.Values...)
	for i := range r.NextRegistration.Values {
		r.NextRegistration.Values[i].Data = append([]byte(nil), r.NextRegistration.Values[i].Data...)
	}
	if r.NextUninstaller != nil {
		copy := *r.NextUninstaller
		r.NextUninstaller = &copy
	}
	for _, p := range []*string{&r.Installation, &r.Profile, &r.Staged, &r.Backup, &r.PreviousPackage, &r.NextPackage, &r.ShortcutPath} {
		if *p == "" {
			return errors.New("upgrade path missing")
		}
		absolute, err := filepath.Abs(*p)
		if err != nil {
			return err
		}
		*p = absolute
	}
	if r.Prepare == nil {
		r.Prepare = func() error { return PrepareNativeUpgrade(r.Installation, r.Profile) }
	}
	if r.CheckStartup == nil {
		r.CheckStartup = func() error { return CheckNativeUpgrade(r.Installation, r.Profile, filepath.Dir(r.Journal.path)) }
	}
	for _, path := range []string{r.Staged, r.Backup, r.Journal.path} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		installation, err := filepath.EvalSymlinks(r.Installation)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(installation, resolved)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("upgrade working data must be outside installation")
		}
	}
	plan, err := PlanUpgrade(r.PreviousPackage, r.NextPackage)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, r.Journal.Snapshot().Plan) {
		return errors.New("upgrade journal does not match packages")
	}
	if _, err = VerifyInstalled(r.Staged, r.NextPackage); err != nil {
		return err
	}
	receipt, err := VerifyBackup(r.Backup, r.PreviousPackage)
	if err != nil {
		return err
	}
	var registration struct {
		SchemaVersion  int                  `json:"schemaVersion"`
		ManifestSHA256 string               `json:"manifestSha256"`
		Registration   RegistrationSnapshot `json:"registration"`
	}
	var shortcut struct {
		SchemaVersion  int              `json:"schemaVersion"`
		ManifestSHA256 string           `json:"manifestSha256"`
		Shortcut       ShortcutSnapshot `json:"shortcut"`
	}
	if err = readUpgradeMetadata(r.Backup, "registration.json", &registration); err != nil {
		return err
	}
	if err = readUpgradeMetadata(r.Backup, "shortcut.json", &shortcut); err != nil {
		return err
	}
	if registration.SchemaVersion != 1 || shortcut.SchemaVersion != 1 || registration.ManifestSHA256 != receipt.ManifestSHA256 || shortcut.ManifestSHA256 != receipt.ManifestSHA256 {
		return errors.New("metadata backup does not match payload")
	}
	originalRegistration, originalShortcut := registration.Registration, shortcut.Shortcut
	for _, snapshot := range []RegistrationSnapshot{originalRegistration, r.NextRegistration} {
		if err = validateRegistrationRestore(r.Installation, r.RegistryKey, snapshot); err != nil {
			return err
		}
	}
	for _, snapshot := range []ShortcutSnapshot{originalShortcut, r.NextShortcut} {
		if err = ValidateShortcutSnapshot(snapshot); err != nil {
			return err
		}
		if relative, err := filepath.Rel(snapshot.Path, r.ShortcutPath); err != nil || relative != "." {
			return errors.New("shortcut backup destination differs")
		}
	}
	var originalUninstaller *File
	if len(receipt.InstallerFiles) == 1 {
		copy := receipt.InstallerFiles[0]
		originalUninstaller = &copy
	}
	if (originalUninstaller == nil) != (r.NextUninstaller == nil) {
		return errors.New("upgrade cannot change installation mode")
	}
	if err = validateUninstallerDescriptor(r.NextUninstaller); err != nil {
		return err
	}
	if err = verifyExecutionUninstaller(r.Staged, r.NextUninstaller); err != nil {
		return err
	}
	bindings := map[string]string{"registration": registrationOperationFingerprint(originalRegistration, r.NextRegistration), "shortcut": shortcutOperationFingerprint(originalShortcut, r.NextShortcut), "uninstaller": uninstallerOperationFingerprint(originalUninstaller, r.NextUninstaller)}
	for name, want := range bindings {
		if got, ok := r.Journal.Snapshot().MetadataFingerprints[name]; ok && got != want {
			return errors.New("upgrade metadata changed since preparation")
		}
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(r.Profile, r.Installation)
	if err != nil {
		return err
	}
	defer lease.Close()
	root, err := openUpgradeRoot(r.Installation)
	if err != nil {
		return err
	}
	defer root.Close()
	verify := func(old bool) error {
		packagePath, reg, link, uninstaller := r.NextPackage, r.NextRegistration, r.NextShortcut, r.NextUninstaller
		if old {
			packagePath, reg, link, uninstaller = r.PreviousPackage, originalRegistration, originalShortcut, originalUninstaller
		}
		if _, err := root.Lstat(".winone-pending"); err == nil {
			_, err = VerifyUpgradePayload(r.Installation, packagePath)
			if err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if _, err = VerifyInstalled(r.Installation, packagePath); err != nil {
				return err
			}
		} else {
			return err
		}
		actual, err := CaptureRegistration(r.Installation, r.RegistryKey)
		if err != nil {
			return err
		}
		if !sameRegistration(actual, reg) {
			return errors.New("registration verification failed")
		}
		actualLink, err := CaptureShortcut(r.ShortcutPath)
		if err != nil {
			return err
		}
		if actualLink.Exists != link.Exists || actualLink.SHA256 != link.SHA256 {
			return errors.New("shortcut verification failed")
		}
		return verifyExecutionUninstaller(r.Installation, uninstaller)
	}
	phase := r.Journal.Snapshot().Phase
	if phase == "committed" || phase == "rolled-back" {
		if err = verify(phase == "rolled-back"); err != nil {
			return err
		}
		if err = clearExecutionMarker(root, r.Journal.Snapshot().ID); err != nil {
			return err
		}
		if phase == "rolled-back" {
			return errors.New("upgrade was rolled back")
		}
		return nil
	}
	if phase == "prepared" {
		if _, err := root.Lstat(".winone-pending"); err == nil {
			if err = ensureExecutionMarker(root, r.Journal.Snapshot().ID, false); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err = verify(true); err != nil {
			return err
		}
		if err = r.Prepare(); err != nil {
			return err
		}
	}
	if err = ensureExecutionMarker(root, r.Journal.Snapshot().ID, phase == "prepared"); err != nil {
		return err
	}
	if phase == "prepared" {
		if err = r.Journal.Transition(r.Journal.Snapshot().Revision, "applying"); err != nil {
			return err
		}
		phase = "applying"
	}
	rollback := func() error {
		if r.Journal.Snapshot().Phase != "restoring" {
			if err := r.Journal.Transition(r.Journal.Snapshot().Revision, "restoring"); err != nil {
				return err
			}
		}
		for i := len(plan.Files) - 1; i >= 0; i-- {
			if r.Journal.Snapshot().FileStates[i] != "pending" {
				if err := RestoreUpgradeFile(r.Installation, r.Backup, r.Journal, i); err != nil {
					return err
				}
			}
		}
		if err := RestoreUpgradeManifest(r.Installation, r.Backup, r.Journal); err != nil {
			return err
		}
		if err := RestoreUpgradeUninstaller(r.Installation, r.Backup, originalUninstaller, r.NextUninstaller, r.Journal); err != nil {
			return err
		}
		if err := RestoreUpgradeRegistration(r.Installation, r.RegistryKey, originalRegistration, r.NextRegistration, r.Journal); err != nil {
			return err
		}
		if err := RestoreUpgradeShortcut(r.ShortcutPath, originalShortcut, r.NextShortcut, r.Journal); err != nil {
			return err
		}
		if err := verify(true); err != nil {
			return err
		}
		if err := r.Journal.Transition(r.Journal.Snapshot().Revision, "rolled-back"); err != nil {
			return err
		}
		return clearExecutionMarker(root, r.Journal.Snapshot().ID)
	}
	if phase == "restoring" {
		if err = rollback(); err != nil {
			return err
		}
		return errors.New("upgrade rolled back")
	}
	apply := func() error {
		if r.Journal.Snapshot().Phase == "applying" {
			for _, name := range []string{"uninstaller", "registration", "shortcut"} {
				if err := r.Journal.BindMetadata(r.Journal.Snapshot().Revision, name, bindings[name]); err != nil {
					return err
				}
			}
			for i := range plan.Files {
				if err := ApplyUpgradeFile(r.Installation, r.Staged, r.Journal, i); err != nil {
					return err
				}
			}
			if err := ApplyUpgradeManifest(r.Installation, r.Staged, r.Journal); err != nil {
				return err
			}
			if err := ApplyUpgradeUninstaller(r.Installation, r.Staged, originalUninstaller, r.NextUninstaller, r.Journal); err != nil {
				return err
			}
			if err := ApplyUpgradeRegistration(r.Installation, r.RegistryKey, originalRegistration, r.NextRegistration, r.Journal); err != nil {
				return err
			}
			if err := ApplyUpgradeShortcut(r.ShortcutPath, originalShortcut, r.NextShortcut, r.Journal); err != nil {
				return err
			}
			if err := r.Journal.Transition(r.Journal.Snapshot().Revision, "checking"); err != nil {
				return err
			}
		}
		if err := verify(false); err != nil {
			return err
		}
		if err := r.CheckStartup(); err != nil {
			return err
		}
		if err := verify(false); err != nil {
			return err
		}
		return r.Journal.Transition(r.Journal.Snapshot().Revision, "committed")
	}
	if err = apply(); err != nil {
		return errors.Join(err, rollback())
	}
	return clearExecutionMarker(root, r.Journal.Snapshot().ID)
}

func readUpgradeMetadata(directory, name string, out any) error {
	root, err := openUpgradeRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := openInstalledFile(root, name)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return errors.New("upgrade metadata too large")
	}
	return configjson.DecodeObject(data, out)
}

func verifyExecutionUninstaller(directory string, want *File) error {
	root, err := openUpgradeRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	hash, err := upgradeFileHash(root, "Uninstall.exe")
	if err != nil {
		return err
	}
	if want == nil {
		if hash != "" {
			return errors.New("unexpected uninstaller")
		}
		return nil
	}
	info, err := root.Stat("Uninstall.exe")
	if err != nil {
		return err
	}
	if hash != want.SHA256 || info.Size() != want.Size {
		return errors.New("uninstaller verification failed")
	}
	return nil
}

func ensureExecutionMarker(root *os.Root, id string, create bool) error {
	file, err := openInstalledFile(root, ".winone-pending")
	if errors.Is(err, os.ErrNotExist) && create {
		file, err = root.OpenFile(".winone-pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(id)
		return errors.Join(writeErr, file.Sync(), file.Close())
	}
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 129))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	if string(data) != id {
		return errors.New("pending marker belongs to another operation")
	}
	return nil
}

func clearExecutionMarker(root *os.Root, id string) error {
	if _, err := root.Lstat(".winone-pending"); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := ensureExecutionMarker(root, id, false); err != nil {
		return err
	}
	return root.Remove(".winone-pending")
}

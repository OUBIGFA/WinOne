//go:build windows

package distribution

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
	"winone/internal/maintenance"
	"winone/internal/startupprobe"
)

// InstallCurrentPackage is the explicit, local setup/repair entry. Unlike an
// automatic upgrade, it must work with a partial installation and without the
// previous installer, manifest, journal, registration or configuration. Only
// the current verified package authorizes writes. Older recovery directories
// are immutable evidence, NOT prerequisites for installing a newer repair.
//
// Before live writes it backs up the actual overwritten bytes (not bytes an old
// manifest claims should exist). Failure rolls back this attempt; termination
// leaves a pending marker and durable backups. Any later installer can repair
// that state from its own payload without executing or trusting old recovery.
func InstallCurrentPackage(r InstallerUpdate) (err error) {
	for _, path := range []string{r.Installation, r.Profile, r.Staged, r.ShortcutPath} {
		if !filepath.IsAbs(path) {
			return errors.New("安装、配置、载荷和快捷方式路径必须是绝对路径")
		}
	}
	if err = os.MkdirAll(r.Installation, 0700); err != nil {
		return err
	}
	r.Installation, err = filepath.EvalSymlinks(r.Installation)
	if err != nil {
		return err
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(r.Profile, r.Installation)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	if err = maintenance.RequireStoppedAt(r.Installation); err != nil {
		return err
	}
	// No directory scan: damaged/foreign/unfinished .update-* records cannot
	// poison an unrelated future installation. Never rename or delete them.
	work, err := os.MkdirTemp(filepath.Dir(r.Installation), filepath.Base(r.Installation)+".install-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("安装未完成：%w；本次备份：%s。可直接重新运行当前或更新的安装包", err, work)
		}
	}()
	reportUpgradeProgress(r.Progress, "verifying")
	archive := filepath.Join(work, "current.zip")
	if err = snapshotInstallerPackage(r.Staged, archive); err != nil {
		return err
	}
	manifest, err := Verify(archive)
	if err != nil {
		return err
	}
	uninstaller, err := installerFile(r.Staged)
	if err != nil {
		return err
	}
	// Source and backup roots use the same bounded, no-link file operations as
	// the automatic updater. Extra files and the live profile are never copied
	// into the package and are never deleted to make an installation succeed.
	target, err := openUpgradeRoot(r.Installation)
	if err != nil {
		return err
	}
	defer target.Close()
	source, err := openUpgradeRoot(r.Staged)
	if err != nil {
		return err
	}
	defer source.Close()
	backupDir := filepath.Join(work, "before")
	if err = os.Mkdir(backupDir, 0700); err != nil {
		return err
	}
	backup, err := openUpgradeRoot(backupDir)
	if err != nil {
		return err
	}
	defer backup.Close()

	registration, err := captureRegistration("", r.RegistryKey, false)
	if err != nil {
		return err
	}
	// Missing/broken registration is repairable. A valid registration naming
	// another directory is not permission to redirect that other installation.
	for _, value := range registration.Values {
		if !strings.EqualFold(value.Name, "InstallLocation") || value.Type != registry.SZ || len(value.Data)%2 != 0 {
			continue
		}
		units := make([]uint16, len(value.Data)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(value.Data[i*2:])
		}
		location := strings.TrimRight(string(utf16.Decode(units)), "\x00")
		if filepath.IsAbs(location) {
			resolved, resolveErr := filepath.EvalSymlinks(location)
			if resolveErr == nil {
				location = resolved
			} else if !errors.Is(resolveErr, os.ErrNotExist) {
				return resolveErr
			}
			if !strings.EqualFold(filepath.Clean(location), filepath.Clean(r.Installation)) {
				return errors.New("安装登记属于另一目录")
			}
		}
	}
	if err = os.MkdirAll(filepath.Dir(r.ShortcutPath), 0700); err != nil {
		return err
	}
	links, err := openUpgradeRoot(filepath.Dir(r.ShortcutPath))
	if err != nil {
		return err
	}
	defer links.Close()
	linkName := filepath.Base(r.ShortcutPath)
	linkHash, err := upgradeFileHash(links, linkName)
	if err != nil {
		return err
	}
	// NSIS creates this shortcut with the actual target before invoking us.
	// Validate the new link, but preserve even a damaged old link as raw bytes.
	newLink, err := CaptureShortcut(filepath.Join(r.Staged, "WinOne.lnk"))
	if err != nil || !newLink.Exists {
		return errors.Join(errors.New("安装包缺少有效快捷方式"), err)
	}
	reportUpgradeProgress(r.Progress, "backing-up")
	files := make([]installerRepairFile, 0, len(manifest.Files)+3)
	owned := map[string]bool{}
	for _, file := range manifest.Files {
		owned[strings.ToLower(file.Path)] = true
		files = append(files, installerRepairFile{Path: file.Path, After: file.SHA256})
	}
	// Remove retired program files only when their bytes still match the old
	// inventory. An unreadable/missing inventory never prevents repair, and
	// modified or unrecognised files are retained, not guessed to be disposable.
	if old, _, readErr := readInstalledManifest(target); readErr == nil && old.SchemaVersion == 1 && old.Product == "WinOne" {
		for _, file := range old.Files {
			key := strings.ToLower(file.Path)
			if owned[key] || !safeName(file.Path) || !allowedHistorical(file.Path) {
				continue
			}
			if hash, hashErr := upgradeFileHash(target, file.Path); hashErr == nil && hash != "" && hash == file.SHA256 {
				owned[key] = true
				files = append(files, installerRepairFile{Path: file.Path})
			}
		}
	}
	for _, name := range []string{"manifest.json", "Uninstall.exe", ".winone-recovery.ini"} {
		hash := ""
		if name != ".winone-recovery.ini" {
			hash, err = upgradeFileHash(source, name)
			if err != nil {
				return err
			}
		}
		files = append(files, installerRepairFile{Path: name, After: hash})
	}
	for i := range files {
		file := &files[i]
		file.Before, err = upgradeFileHash(target, file.Path)
		if err != nil {
			return fmt.Errorf("备份 %s：%w", file.Path, err)
		}
		if file.Before != "" && file.Before != file.After {
			if err = replaceUpgradeFile(backup, target, file.Path, file.Path, file.Before, ""); err != nil {
				return err
			}
		}
	}
	markerHash, err := upgradeFileHash(target, ".winone-pending")
	if err != nil {
		return err
	}
	if markerHash != "" {
		if err = replaceUpgradeFile(backup, target, ".winone-pending", ".winone-pending", markerHash, ""); err != nil {
			return err
		}
	}
	if linkHash != "" {
		if err = replaceUpgradeFile(backup, links, "previous-shortcut", linkName, linkHash, ""); err != nil {
			return err
		}
	}
	receipt := struct {
		SchemaVersion                                                           int
		Installation, Profile, Package, Shortcut, ShortcutBefore, PendingBefore string
		Registration                                                            RegistrationSnapshot
		Files                                                                   []installerRepairFile
	}{1, r.Installation, r.Profile, manifestFingerprint(manifest), r.ShortcutPath, linkHash, markerHash, registration, files}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err = writeBackupMetadata(work, "before.json", raw); err != nil {
		return err
	}
	// Installing program bytes does not migrate or validate user settings. No
	// old Host/WinTray is launched: it may be missing or too old for this profile.
	// Runtime leases + stopped-process checks guard resources; startup probes
	// use only new binaries and isolated scratch, never the user's configuration.
	if r.Prepare != nil {
		if err = r.Prepare(); err != nil {
			return err
		}
	}
	marker := filepath.Base(work)
	if err = os.WriteFile(filepath.Join(work, ".winone-pending"), []byte(marker), 0600); err != nil {
		return err
	}
	workRoot, err := openUpgradeRoot(work)
	if err != nil {
		return err
	}
	defer workRoot.Close()
	pendingHash, err := upgradeFileHash(workRoot, ".winone-pending")
	if err != nil {
		return err
	}
	if err = replaceUpgradeFile(target, workRoot, ".winone-pending", ".winone-pending", pendingHash, markerHash); err != nil {
		return err
	}
	desired := installerRepairRegistration(registration, r.Installation, manifest.Version)
	touched, registrationTouched, linkTouched := 0, false, false
	defer func() {
		if err == nil {
			return
		}
		reportUpgradeProgress(r.Progress, "rolling-back")
		// The receipt is durable before writes. In-process rollback restores only
		// this attempt, checking hashes to avoid overwriting an external change.
		rollbackErr := func() error {
			if linkTouched {
				if e := restoreInstallerRepairFile(links, backup, linkName, "previous-shortcut", linkHash, newLink.SHA256); e != nil {
					return e
				}
			}
			if registrationTouched {
				current, e := captureRegistration("", r.RegistryKey, false)
				if e != nil {
					return e
				}
				if !sameRegistration(current, registration) {
					if !registrationIntermediate(current, registration, desired) {
						return errors.New("安装登记被其他程序修改，保留备份")
					}
					if e = replaceRegistrationValues(r.RegistryKey, registration, current); e != nil {
						return e
					}
				}
			}
			for i := touched - 1; i >= 0; i-- {
				file := files[i]
				if e := restoreInstallerRepairFile(target, backup, file.Path, file.Path, file.Before, file.After); e != nil {
					return e
				}
			}
			plan := UpgradePlan{}
			for _, file := range files {
				plan.Files = append(plan.Files, UpgradeFile{Path: file.Path, PreviousSHA256: file.Before, NextSHA256: file.After})
			}
			if e := removeRolledBackDirectories(target, plan); e != nil {
				return e
			}
			return restoreInstallerRepairFile(target, backup, ".winone-pending", ".winone-pending", markerHash, pendingHash)
		}()
		err = errors.Join(err, rollbackErr)
	}()
	reportUpgradeProgress(r.Progress, "applying")
	for i, file := range files {
		touched = i + 1 // includes an attempted operation that may have written
		if err = applyInstallerRepairFile(target, source, file); err != nil {
			return fmt.Errorf("安装 %s：%w", file.Path, err)
		}
	}
	if _, err = VerifyUpgradePayload(r.Installation, archive); err != nil {
		return err
	}
	reportUpgradeProgress(r.Progress, "checking-startup")
	if r.CheckStartup != nil {
		err = r.CheckStartup()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err = startupprobe.CheckWorkers(ctx, r.Installation, work)
		if err == nil {
			err = startupprobe.CheckDesktop(ctx, filepath.Join(r.Installation, "WinOne.exe"), work)
		}
	}
	if err != nil {
		return err
	}
	if _, err = VerifyUpgradePayload(r.Installation, archive); err != nil {
		return err
	}
	if err = verifyExecutionUninstaller(r.Installation, &uninstaller); err != nil {
		return err
	}
	registrationTouched = true
	if err = replaceRegistrationValues(r.RegistryKey, desired, registration); err != nil {
		return err
	}
	linkTouched = true
	if err = replaceUpgradeFile(links, source, linkName, "WinOne.lnk", newLink.SHA256, linkHash); err != nil {
		return err
	}
	actual, err := CaptureRegistration(r.Installation, r.RegistryKey)
	if err != nil || !sameRegistration(actual, desired) {
		return errors.Join(errors.New("安装登记校验失败"), err)
	}
	actualLink, err := upgradeFileHash(links, linkName)
	if err != nil || actualLink != newLink.SHA256 {
		return errors.Join(errors.New("快捷方式校验失败"), err)
	}
	if err = maintenance.RequireStoppedAt(r.Installation); err != nil {
		return err
	}
	if err = writeBackupMetadata(work, "complete.json", []byte(`{"schemaVersion":1,"verified":true}`)); err != nil {
		return err
	}
	return clearExecutionMarker(target, marker)
}

type installerRepairFile struct {
	Path, Before, After string
}

func applyInstallerRepairFile(target, source *os.Root, file installerRepairFile) error {
	current, err := upgradeFileHash(target, file.Path)
	if err != nil {
		return err
	}
	if current != file.Before {
		return errors.New("文件在备份后发生变化")
	}
	if current == file.After {
		return nil
	}
	if file.After == "" {
		return target.Remove(filepath.FromSlash(file.Path))
	}
	return replaceUpgradeFile(target, source, file.Path, file.Path, file.After, current)
}

func restoreInstallerRepairFile(target, backup *os.Root, name, backupName, before, after string) error {
	current, err := upgradeFileHash(target, name)
	if err != nil {
		return err
	}
	if current == before {
		return nil
	}
	if current != after {
		return fmt.Errorf("%s 在安装过程中被其他程序修改，保留备份", name)
	}
	if before == "" {
		return target.Remove(filepath.FromSlash(name))
	}
	return replaceUpgradeFile(target, backup, name, backupName, before, current)
}

func installerRepairRegistration(before RegistrationSnapshot, installation, version string) RegistrationSnapshot {
	desired := before
	desired.Exists = true
	desired.Values = append([]RegistrationValue(nil), before.Values...)
	for name, text := range map[string]string{"DisplayName": "WinOne", "DisplayVersion": version, "InstallLocation": installation, "UninstallString": `"` + filepath.Join(installation, "Uninstall.exe") + `"`} {
		units := utf16.Encode([]rune(text + "\x00"))
		data := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(data[i*2:], unit)
		}
		value := RegistrationValue{Name: name, Type: registry.SZ, Data: data}
		found := false
		for i := range desired.Values {
			if strings.EqualFold(desired.Values[i].Name, name) {
				desired.Values[i] = value
				found = true
				break
			}
		}
		if !found {
			desired.Values = append(desired.Values, value)
		}
	}
	return desired
}

//go:build windows

package distribution

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"winone/internal/maintenance"
)

// InstallerUpdate uses the same transactional executor for local upgrades and
// same-version reinstalls. Its work directory is a sibling of the resolved install.
type InstallerUpdate struct {
	Installation, Profile, Staged, RegistryKey, ShortcutPath string
	Prepare, CheckStartup, CheckConfiguration                func() error
	// Progress reports the work currently starting; it never implies success
	// or a fraction of total duration. The caller owns completion presentation.
	Progress func(string)
}

func RunInstallerUpdate(r InstallerUpdate) error {
	reportUpgradeProgress(r.Progress, "verifying")
	installation, err := filepath.EvalSymlinks(r.Installation)
	if err != nil {
		return err
	}
	installation, err = filepath.Abs(installation)
	if err != nil {
		return err
	}
	r.Installation = installation
	// Resolve the installation once. Programs can be a cross-volume symlink;
	// all journal, backup and file operations must use its real directory.
	work := installation + ".update"
	lease, err := maintenance.AcquireMaintenanceForInstallation(r.Profile, installation)
	if err != nil {
		return err
	}
	work, err = installerWorkDirectory(r, work)
	if err != nil {
		return errors.Join(err, lease.Close())
	}
	request, err := prepareInstallerUpdate(r, work)
	closeErr := lease.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("准备覆盖安装：%w；恢复目录：%s", err, work)
	}
	request.Prepare, request.CheckStartup, request.CheckConfiguration = r.Prepare, r.CheckStartup, r.CheckConfiguration
	request.Progress = r.Progress
	// ExecuteUpgrade reacquires the lease and verifies the live payload against
	// its backup before writing. Changes during handoff therefore fail closed.
	if err = ExecuteUpgrade(request); err != nil {
		return fmt.Errorf("覆盖安装：%w；恢复目录：%s", err, work)
	}
	return nil
}

type installerUpdateOwner struct {
	Installation, Profile, RegistryKey, ShortcutPath string
}

func prepareInstallerUpdate(r InstallerUpdate, work string) (UpgradeExecution, error) {
	var result UpgradeExecution
	if err := maintenance.RequireStoppedAt(r.Installation); err != nil {
		return result, err
	}
	owner := installerUpdateOwner{r.Installation, filepath.Clean(r.Profile), r.RegistryKey, filepath.Clean(r.ShortcutPath)}
	journalPath := filepath.Join(work, "journal.json")
	if info, err := os.Lstat(work); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return result, errors.New("恢复目录不是普通文件夹")
		}
		var saved installerUpdateOwner
		if err := readUpgradeMetadata(work, "owner.json", &saved); err != nil {
			return result, err
		}
		if saved != owner {
			return result, errors.New("恢复目录属于另一安装操作")
		}
		journal, err := OpenUpgradeJournal(journalPath)
		phase := "prepared"
		if err != nil {
			// Interrupted preparation has no live writes. Keep its files as a
			// separate backup and allow a fresh attempt only with no live marker.
			_, markerErr := os.Lstat(filepath.Join(r.Installation, ".winone-pending"))
			if !errors.Is(err, os.ErrNotExist) || !errors.Is(markerErr, os.ErrNotExist) {
				return result, fmt.Errorf("恢复记录未完成，请保留 %s：%w", work, err)
			}
			phase = "rolled-back"
		} else {
			phase = journal.Snapshot().Phase
		}
		if phase != "committed" && phase != "rolled-back" {
			if _, err := VerifyInstalled(r.Staged, filepath.Join(work, "next.zip")); err != nil {
				return result, errors.New("请使用上次的安装包继续未完成操作")
			}
			current, err := installerFile(r.Staged)
			if err != nil {
				return result, err
			}
			saved, err := installerFile(filepath.Join(work, "payload"))
			if err != nil {
				return result, err
			}
			if current != saved {
				return result, errors.New("恢复卸载程序与安装包不匹配")
			}
			return loadInstallerExecution(r, work, journal)
		}
		return result, errors.New("恢复事务已结束，请使用新的恢复目录")
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if err := os.Mkdir(work, 0700); err != nil {
		return result, err
	}
	data, err := json.Marshal(owner)
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(filepath.Join(work, "owner.json"), data, 0600); err != nil {
		return result, err
	}
	previous, next := filepath.Join(work, "previous.zip"), filepath.Join(work, "next.zip")
	reportUpgradeProgress(r.Progress, "backing-up")
	if err = snapshotInstallerPackage(r.Installation, previous); err != nil {
		return result, err
	}
	if err = snapshotInstallerPackage(r.Staged, next); err != nil {
		return result, err
	}
	plan, err := PlanInstallation(previous, next)
	if err != nil {
		return result, err
	}
	staged, backup := filepath.Join(work, "payload"), filepath.Join(work, "backup")
	if err = extractInstallerPackage(next, staged); err != nil {
		return result, err
	}
	uninstaller, err := installerFile(r.Staged)
	if err != nil {
		return result, err
	}
	raw, err := os.ReadFile(filepath.Join(r.Staged, "Uninstall.exe"))
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(filepath.Join(staged, "Uninstall.exe"), raw, 0700); err != nil {
		return result, err
	}
	actual, err := installerFile(staged)
	if err != nil || actual != uninstaller {
		return result, errors.New("暂存卸载程序校验失败")
	}
	if _, err = BackupInstalled(r.Installation, previous, backup); err != nil {
		return result, err
	}
	if err = BackupRegistration(r.Installation, previous, backup, r.RegistryKey); err != nil {
		return result, err
	}
	if err = BackupShortcut(previous, backup, r.ShortcutPath); err != nil {
		return result, err
	}
	journal, err := CreateUpgradeJournal(journalPath, plan)
	if err != nil {
		return result, err
	}
	return loadInstallerExecution(r, work, journal)
}

func loadInstallerExecution(r InstallerUpdate, work string, journal *UpgradeJournalStore) (UpgradeExecution, error) {
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
	backup, staged := filepath.Join(work, "backup"), filepath.Join(work, "payload")
	if err := readUpgradeMetadata(backup, "registration.json", &registration); err != nil {
		return UpgradeExecution{}, err
	}
	if err := readUpgradeMetadata(backup, "shortcut.json", &shortcut); err != nil {
		return UpgradeExecution{}, err
	}
	if !registration.Registration.Exists {
		return UpgradeExecution{}, errors.New("目标没有有效的 WinOne 安装登记")
	}
	units := utf16.Encode([]rune(journal.Snapshot().Plan.NextVersion + "\x00"))
	version := make([]byte, len(units)*2)
	for i, v := range units {
		binary.LittleEndian.PutUint16(version[i*2:], v)
	}
	found := false
	for i, v := range registration.Registration.Values {
		if strings.EqualFold(v.Name, "DisplayVersion") {
			registration.Registration.Values[i].Type = registry.SZ
			registration.Registration.Values[i].Data = version
			found = true
		}
	}
	if !found {
		return UpgradeExecution{}, errors.New("安装登记缺少版本")
	}
	uninstaller, err := installerFile(staged)
	if err != nil {
		return UpgradeExecution{}, err
	}
	return UpgradeExecution{Installation: r.Installation, Profile: r.Profile, Staged: staged, Backup: backup, PreviousPackage: filepath.Join(work, "previous.zip"), NextPackage: filepath.Join(work, "next.zip"), RegistryKey: r.RegistryKey, ShortcutPath: r.ShortcutPath, NextRegistration: registration.Registration, NextShortcut: shortcut.Shortcut, NextUninstaller: &uninstaller, Journal: journal}, nil
}

func installerFile(directory string) (File, error) {
	root, err := openUpgradeRoot(directory)
	if err != nil {
		return File{}, err
	}
	defer root.Close()
	file, err := openInstalledFile(root, "Uninstall.exe")
	if err != nil {
		return File{}, err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return File{}, err
	}
	if size == 0 || size > maxFileBytes {
		return File{}, errors.New("卸载程序大小无效")
	}
	return File{Path: "Uninstall.exe", Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

// Reconstruct only manifest-owned bytes so older installs need no original ZIP.
// Validate names and limits before reads, then run the normal package verifier.
func snapshotInstallerPackage(directory, destination string) error {
	root, err := openUpgradeRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	manifest, raw, err := readInstalledManifest(root)
	if err != nil {
		return err
	}
	if len(manifest.Files) > 8191 {
		return errors.New("安装清单过大")
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	writeErr := func() error {
		header := &zip.FileHeader{Name: "manifest.json", Method: zip.Store}
		stream, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err = stream.Write(raw); err != nil {
			return err
		}
		total := int64(0)
		for _, record := range manifest.Files {
			if !safeName(record.Path) || record.Path == "manifest.json" || record.Size < 0 || record.Size > maxFileBytes {
				return errors.New("安装清单路径或大小无效")
			}
			total += record.Size
			if total > maxTotalBytes {
				return errors.New("安装文件超过大小限制")
			}
			source, err := openInstalledFile(root, record.Path)
			if err != nil {
				return err
			}
			stream, err := archive.CreateHeader(&zip.FileHeader{Name: record.Path, Method: zip.Store})
			if err != nil {
				source.Close()
				return err
			}
			size, copyErr := io.Copy(stream, io.LimitReader(source, record.Size+1))
			err = errors.Join(copyErr, source.Close())
			if err != nil {
				return err
			}
			if size != record.Size {
				return errors.New("安装文件大小变化")
			}
		}
		return nil
	}()
	if err = errors.Join(writeErr, archive.Close(), file.Sync(), file.Close()); err != nil {
		return err
	}
	if _, err = Verify(destination); err != nil {
		return err
	}
	_, err = VerifyInstalled(directory, destination)
	return err
}

func extractInstallerPackage(archive, directory string) error {
	if _, err := Verify(archive); err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		name := filepath.FromSlash(entry.Name)
		if err = root.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		target, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(target, source)
		if err = errors.Join(copyErr, source.Close(), target.Close()); err != nil {
			return err
		}
	}
	_, err = VerifyInstalled(directory, archive)
	return err
}

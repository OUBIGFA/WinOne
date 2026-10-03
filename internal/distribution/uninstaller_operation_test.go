package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func uninstallerFixture(t *testing.T, root, text string) *File {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "Uninstall.exe"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(text))
	return &File{Path: "Uninstall.exe", Size: int64(len(text)), SHA256: hex.EncodeToString(digest[:])}
}

func TestUpgradeUninstallerAppliesAndRestores(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	original := uninstallerFixture(t, installation, "old uninstaller")
	uninstallerFixture(t, backup, "old uninstaller")
	next := uninstallerFixture(t, staged, "new uninstaller")
	if err := ApplyUpgradeUninstaller(installation, staged, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if journal.Snapshot().MetadataStates["uninstaller"] != "applied" {
		t.Fatal("apply not recorded")
	}
	if err := journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreUpgradeUninstaller(installation, backup, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(installation, "Uninstall.exe")); err != nil || string(data) != "old uninstaller" {
		t.Fatal(string(data), err)
	}
}

func TestUpgradeUninstallerOriginalAbsenceAndExternalChange(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	next := uninstallerFixture(t, staged, "new uninstaller")
	if err := ApplyUpgradeUninstaller(installation, staged, nil, next, journal); err != nil {
		t.Fatal(err)
	}
	if err := journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	uninstallerFixture(t, installation, "outside change")
	if err := RestoreUpgradeUninstaller(installation, backup, nil, next, journal); err == nil {
		t.Fatal("external change overwritten")
	}
	uninstallerFixture(t, installation, "new uninstaller")
	if err := RestoreUpgradeUninstaller(installation, backup, nil, next, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installation, "Uninstall.exe")); !os.IsNotExist(err) {
		t.Fatal("original absence not restored", err)
	}
}

func TestUpgradeUninstallerResumesUnrecordedWrite(t *testing.T) {
	installation, staged, _, journal := fileOperationFixture(t)
	original := uninstallerFixture(t, installation, "old")
	next := uninstallerFixture(t, staged, "new")
	if err := journal.BindMetadata(journal.Snapshot().Revision, "uninstaller", uninstallerOperationFingerprint(original, next)); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordMetadata(journal.Snapshot().Revision, "uninstaller", "writing"); err != nil {
		t.Fatal(err)
	}
	uninstallerFixture(t, installation, "new")
	reopened, err := OpenUpgradeJournal(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeUninstaller(installation, staged, original, next, reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().MetadataStates["uninstaller"] != "applied" {
		t.Fatal("completion not recovered")
	}
}

func TestRealUninstallerFilesRoundTrip(t *testing.T) {
	oldPath, newPath := os.Getenv("WINONE_OLD_UNINSTALLER"), os.Getenv("WINONE_NEW_UNINSTALLER")
	if oldPath == "" || newPath == "" {
		t.Skip("set WINONE_OLD_UNINSTALLER and WINONE_NEW_UNINSTALLER")
	}
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(oldData) == string(newData) {
		t.Fatal("fixture uninstallers must differ")
	}
	installation, staged, backup, journal := fileOperationFixture(t)
	original := uninstallerFixture(t, installation, string(oldData))
	uninstallerFixture(t, backup, string(oldData))
	next := uninstallerFixture(t, staged, string(newData))
	if err = ApplyUpgradeUninstaller(installation, staged, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeUninstaller(installation, backup, original, next, journal); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(installation, "Uninstall.exe"))
	if err != nil || string(data) != string(oldData) {
		t.Fatal("real uninstaller bytes not restored", err)
	}
}

func TestPortableUpgradeKeepsUninstallerAbsent(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	if err := ApplyUpgradeUninstaller(installation, staged, nil, nil, journal); err != nil {
		t.Fatal(err)
	}
	if journal.Snapshot().MetadataStates["uninstaller"] != "unchanged" {
		t.Fatal("absence not recorded")
	}
	if err := journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreUpgradeUninstaller(installation, backup, nil, nil, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installation, "Uninstall.exe")); !os.IsNotExist(err) {
		t.Fatal("portable install gained uninstaller", err)
	}
}

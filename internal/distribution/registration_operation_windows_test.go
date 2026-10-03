//go:build windows

package distribution

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpgradeRegistrationResumesPartialValues(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1")
	}
	dir := t.TempDir()
	path := fmt.Sprintf(`Software\WinOneInstallerTests\operation-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	if err = key.SetStringValue("InstallLocation", dir); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("DisplayVersion", "old"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetDWordValue("NoModify", 1); err != nil {
		t.Fatal(err)
	}
	original, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("DisplayVersion", "new"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetDWordValue("NoModify", 0); err != nil {
		t.Fatal(err)
	}
	next, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = RestoreRegistration(dir, path, original, next); err != nil {
		t.Fatal(err)
	}
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = journal.BindMetadata(journal.Snapshot().Revision, "registration", registrationOperationFingerprint(original, next)); err != nil {
		t.Fatal(err)
	}
	if err = journal.RecordMetadata(journal.Snapshot().Revision, "registration", "writing"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("DisplayVersion", "new"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("External", "keep"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeRegistration(dir, path, original, next, journal); err == nil {
		t.Fatal("external value accepted")
	}
	if value, _, err := key.GetStringValue("External"); err != nil || value != "keep" {
		t.Fatal("external value changed", err)
	}
	if err = key.DeleteValue("External"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeRegistration(dir, path, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if state, err := CaptureRegistration(dir, path); err != nil || !sameRegistration(state, next) {
		t.Fatal(state, err)
	}
	otherOriginal := original
	otherOriginal.Values = append([]RegistrationValue(nil), original.Values...)
	for i := range otherOriginal.Values {
		if otherOriginal.Values[i].Name == "NoModify" {
			otherOriginal.Values[i].Data = []byte{2, 0, 0, 0}
		}
	}
	if err = ApplyUpgradeRegistration(dir, path, otherOriginal, next, journal); err == nil {
		t.Fatal("bound registry snapshot replaced")
	}
	if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = journal.RecordMetadata(journal.Snapshot().Revision, "registration", "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("DisplayVersion", "old"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeRegistration(dir, path, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if state, err := CaptureRegistration(dir, path); err != nil || !sameRegistration(state, original) {
		t.Fatal(state, err)
	}
	if journal.Snapshot().MetadataStates["registration"] != "restored" {
		t.Fatal("restore not recorded")
	}
}

func TestUpgradeRegistrationResumesBeforeLocationValueExists(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1")
	}
	dir := t.TempDir()
	path := fmt.Sprintf(`Software\WinOneInstallerTests\new-operation-%d`, time.Now().UnixNano())
	original, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	if err = key.SetStringValue("InstallLocation", dir); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("DisplayName", "WinOne"); err != nil {
		t.Fatal(err)
	}
	next, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a new key interrupted before its location value is written.
	if err = key.DeleteValue("InstallLocation"); err != nil {
		t.Fatal(err)
	}
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	if err = journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeRegistration(dir, path, original, next, journal); err == nil {
		t.Fatal("partial key accepted without intent")
	}
	if err = journal.BindMetadata(journal.Snapshot().Revision, "registration", registrationOperationFingerprint(original, next)); err != nil {
		t.Fatal(err)
	}
	if err = journal.RecordMetadata(journal.Snapshot().Revision, "registration", "writing"); err != nil {
		t.Fatal(err)
	}
	if err = ApplyUpgradeRegistration(dir, path, original, next, journal); err != nil {
		t.Fatal(err)
	}
	if state, err := CaptureRegistration(dir, path); err != nil || !sameRegistration(state, next) {
		t.Fatal(state, err)
	}
}

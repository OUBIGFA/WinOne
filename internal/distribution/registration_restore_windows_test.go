//go:build windows

package distribution

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"testing"
	"time"
)

func TestRegistrationRestoreRejectsInvalidSnapshotsBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	wide, err := windows.UTF16FromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len(wide)*2)
	for i, v := range wide {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	path := `Software\WinOneInstallerTests\validation`
	base := RegistrationSnapshot{SchemaVersion: 1, KeyPath: path, Exists: true, Values: []RegistrationValue{{Name: "InstallLocation", Type: registry.SZ, Data: data}, {Name: "NoModify", Type: registry.DWORD, Data: []byte{1, 0, 0, 0}}}}
	if err := validateRegistrationRestore(dir, path, base); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"schema", "key", "duplicate", "type", "missing-location"} {
		snapshot := base
		snapshot.Values = append([]RegistrationValue(nil), base.Values...)
		switch scenario {
		case "schema":
			snapshot.SchemaVersion = 0
		case "key":
			snapshot.KeyPath += `\other`
		case "duplicate":
			snapshot.Values = append(snapshot.Values, RegistrationValue{Name: "nomodify", Type: registry.DWORD, Data: []byte{1, 0, 0, 0}})
		case "type":
			snapshot.Values[1].Data = []byte{1}
		case "missing-location":
			snapshot.Values = snapshot.Values[1:]
		}
		if err := validateRegistrationRestore(dir, path, snapshot); err == nil {
			t.Fatal("invalid snapshot accepted", scenario)
		}
	}
}

func TestRegistrationRestorePreservesTypesAndDetectsExternalChanges(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1")
	}
	dir := t.TempDir()
	path := fmt.Sprintf(`Software\WinOneInstallerTests\restore-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	if err = key.SetStringValue("InstallLocation", dir); err != nil {
		t.Fatal(err)
	}
	if err = key.SetDWordValue("NoModify", 1); err != nil {
		t.Fatal(err)
	}
	original, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("NoModify", "changed type"); err != nil {
		t.Fatal(err)
	}
	if err = key.SetBinaryValue("NewValue", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	written, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = RestoreRegistration(dir, path, original, written); err != nil {
		t.Fatal(err)
	}
	if value, _, err := key.GetIntegerValue("NoModify"); err != nil || value != 1 {
		t.Fatal(value, err)
	}
	if _, _, err := key.GetValue("NewValue", nil); err != registry.ErrNotExist {
		t.Fatal("new value retained", err)
	}
	if err = RestoreRegistration(dir, path, original, written); err != nil {
		t.Fatal("retry failed", err)
	}
	if err = key.SetStringValue("External", "keep"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreRegistration(dir, path, original, written); err == nil {
		t.Fatal("external change accepted")
	}
	if value, _, err := key.GetStringValue("External"); err != nil || value != "keep" {
		t.Fatal("external data changed", err)
	}
}

func TestRegistrationRestoreOriginalAbsence(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1")
	}
	dir := t.TempDir()
	path := fmt.Sprintf(`Software\WinOneInstallerTests\restore-absent-%d`, time.Now().UnixNano())
	original, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	if err = key.SetStringValue("InstallLocation", dir); err != nil {
		key.Close()
		t.Fatal(err)
	}
	key.Close()
	written, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = RestoreRegistration(dir, path, original, written); err != nil {
		t.Fatal(err)
	}
	if err = RestoreRegistration(dir, path, original, written); err != nil {
		t.Fatal(err)
	}
	state, err := CaptureRegistration(dir, path)
	if err != nil || state.Exists {
		t.Fatal(state, err)
	}
}

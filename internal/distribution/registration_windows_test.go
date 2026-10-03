//go:build windows

package distribution

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistrationSnapshotPreservesTypesAndRejectsWrongOwner(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1 for isolated registry writes")
	}
	dir, reference := installedFixture(t)
	path := fmt.Sprintf(`Software\WinOneInstallerTests\snapshot-%d`, time.Now().UnixNano())
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
	if err = key.SetBinaryValue("TestData", []byte{0, 1, 255}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureRegistration(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Exists || len(snapshot.Values) != 3 {
		t.Fatal(snapshot)
	}
	for _, value := range snapshot.Values {
		if value.Name == "NoModify" && (value.Type != registry.DWORD || binary.LittleEndian.Uint32(value.Data) != 1) {
			t.Fatal(value)
		}
		if value.Name == "TestData" && (value.Type != registry.BINARY || string(value.Data) != string([]byte{0, 1, 255})) {
			t.Fatal(value)
		}
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(dir, reference, backup); err != nil {
		t.Fatal(err)
	}
	if err := BackupRegistration(dir, reference, backup, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(backup, "registration.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ManifestSHA256 string               `json:"manifestSha256"`
		Registration   RegistrationSnapshot `json:"registration"`
	}
	if err = json.Unmarshal(data, &saved); err != nil || len(saved.ManifestSHA256) != 64 || !saved.Registration.Exists || len(saved.Registration.Values) != 3 {
		t.Fatal("snapshot not persisted", err)
	}
	if err := BackupRegistration(dir, reference, backup, path); err == nil {
		t.Fatal("existing registration snapshot overwritten")
	}
	if _, err = CaptureRegistration(t.TempDir(), path); err == nil {
		t.Fatal("unrelated registration accepted")
	}
	if err = key.SetBinaryValue("Oversize", make([]byte, 65537)); err != nil {
		t.Fatal(err)
	}
	if _, err = CaptureRegistration(dir, path); err == nil {
		t.Fatal("oversized registry value accepted")
	}
}

func TestRegistrationSnapshotRecordsAbsence(t *testing.T) {
	snapshot, err := CaptureRegistration(t.TempDir(), fmt.Sprintf(`Software\WinOneInstallerTests\missing-%d`, time.Now().UnixNano()))
	if err != nil || snapshot.Exists || len(snapshot.Values) != 0 {
		t.Fatal(snapshot, err)
	}
}

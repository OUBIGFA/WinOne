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

// The wizard may outlive an uninstall. Its startup snapshot must not force a
// nonexistent old folder through the upgrade-only path when Install is clicked.
func TestNativeInstallerRefreshesRegistrationBeforeWriting(t *testing.T) {
	compiler, host := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_HOST_EXE")
	if compiler == "" || host == "" {
		t.Skip("explicit isolated NSIS fixture required")
	}
	source, license := fixture(t)
	raw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "WinOne.Host.exe"), raw, 0700); err != nil {
		t.Fatal(err)
	}
	if err = RecordBuild(source, license, "0.0.1"); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "fixture.zip")
	if err = Build(source, license, archive, "0.0.1"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	installation, profile, shortcut := filepath.Join(root, "installed"), filepath.Join(root, "profile"), filepath.Join(root, "WinOne.lnk")
	if err = os.Mkdir(installation, 0700); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf(`Software\WinOneInstallerTests\refresh-%d`, time.Now().UnixNano())
	entry, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	if err = entry.SetStringValue("InstallLocation", installation); err != nil {
		t.Fatal(err)
	}
	entry.Close()
	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, key) })
	setup, _, _ := compileNativeUpgradeInstaller(t, compiler, archive, key, profile, shortcut)
	runNativeSetup(t, setup, "/S", "/FAILPOINT=registration-removed-before-install", "/D="+installation)
	if _, err = VerifyInstalled(installation, archive); err != nil {
		t.Fatal(err)
	}
	registration, err := CaptureRegistration(installation, key)
	if err != nil || !registration.Exists {
		t.Fatal("fresh installation missing", err)
	}
}

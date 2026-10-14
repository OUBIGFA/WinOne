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

func TestNativeInstallerPreservesMachineHintAcrossUninstallAndReinstall(t *testing.T) {
	compiler, host, wintray := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_HOST_EXE"), os.Getenv("WINONE_WINTRAY_EXE")
	if compiler == "" || host == "" || wintray == "" {
		t.Skip("explicit isolated NSIS/Host fixture required")
	}
	source, license := fixture(t)
	raw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "WinOne.Host.exe"), raw, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(wintray)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "WinOne.WinTray.exe"), raw, 0700); err != nil {
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
	key := fmt.Sprintf(`Software\WinOneInstallerTests\onboarding-%d`, time.Now().UnixNano())
	t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, key) })
	setup, uninstall, _ := compileNativeUpgradeInstaller(t, compiler, archive, key, profile, shortcut)
	t.Log("initial install")
	runNativeSetup(t, setup, "/S", "/D="+installation)
	marker := filepath.Join(root, "machine", "WinOne", "onboarding", "result-send-hint-v1.once")
	if _, err = os.Stat(filepath.Dir(marker)); err != nil {
		t.Fatalf("installer did not prepare the compiled isolated directory: %v", err)
	}
	if err = os.WriteFile(marker, []byte("shown"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("normal uninstall")
	runNativeSetup(t, uninstall, "/S", "_?="+installation)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "shown" {
		t.Fatal("normal uninstall removed machine first-use state", err)
	}
	t.Log("reinstall")
	runNativeSetup(t, setup, "/S", "/D="+installation)
	if data, err := os.ReadFile(marker); err != nil || string(data) != "shown" {
		t.Fatal("reinstall reset machine first-use state", err)
	}
}

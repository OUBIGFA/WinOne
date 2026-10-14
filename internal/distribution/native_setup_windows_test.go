//go:build windows

package distribution

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the launcher with a real NSIS installer and uninstaller. A quoted
// trailing directory must not silently select a different installation target.
func TestNativeSetupPreservesDirectoryArguments(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE for the isolated NSIS argument check")
	}
	work := t.TempDir()
	setup := filepath.Join(work, "setup.exe")
	fallback := filepath.Join(work, "unused default")
	destination := filepath.Join(work, "安装 path with spaces")
	source := filepath.Join(work, "arguments.nsi")
	script := `Unicode true
Name "WinOne argument probe"
OutFile "${SETUP_EXE}"
InstallDir "${DEFAULT_DIR}"
RequestExecutionLevel user
SilentInstall silent
SilentUnInstall silent
Section
  CreateDirectory "$INSTDIR"
  FileOpen $0 "$INSTDIR\installed.txt" w
  FileWrite $0 "installed"
  FileClose $0
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd
Section "Uninstall"
  Delete "$INSTDIR\installed.txt"
SectionEnd
`
	if err := os.WriteFile(source, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, compiler, "/V2", "/INPUTCHARSET", "UTF8",
		"/DSETUP_EXE="+strings.ReplaceAll(setup, "$", "$$"),
		"/DDEFAULT_DIR="+strings.ReplaceAll(fallback, "$", "$$"), source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile directory probe: %v: %s", err, output)
	}
	runNativeSetup(t, setup, "/S", "/D="+destination)
	marker := filepath.Join(destination, "installed.txt")
	if data, err := os.ReadFile(marker); err != nil || string(data) != "installed" {
		t.Fatalf("installer did not use the requested directory: %q, %v", data, err)
	}
	if _, err := os.Stat(fallback); !os.IsNotExist(err) {
		t.Fatalf("installer touched its fallback directory: %v", err)
	}
	uninstaller := filepath.Join(work, "run-uninstall.exe")
	data, err := os.ReadFile(filepath.Join(destination, "Uninstall.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uninstaller, data, 0700); err != nil {
		t.Fatal(err)
	}
	runNativeSetup(t, uninstaller, "/S", "_?="+destination)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("uninstaller did not use the requested directory: %v", err)
	}
}

//go:build windows

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNSISExportsExactInstalledUninstaller(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	installation := filepath.Join(work, "installed")
	exported := filepath.Join(work, "exported $ & uninstall.exe")
	t.Setenv("WINONE_UNINSTALLER_EXPORT", exported)
	script := fmt.Sprintf(`Unicode true
Name "Uninstaller export test"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
!include "%s"
Section
CreateDirectory "%s"
WriteUninstaller "%s\Uninstall.exe"
SectionEnd
Section "Uninstall"
SetErrorLevel 0
SectionEnd
`, filepath.Join(work, "setup.exe"), filepath.Join(root, "build", "windows", "export-uninstaller.nsh"), installation, installation)
	path := filepath.Join(work, "export.nsi")
	if err = os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	compile := func() error {
		output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}
	if err = compile(); err != nil {
		t.Fatal(err)
	}
	if err = exec.Command(filepath.Join(work, "setup.exe")).Run(); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(installation, "Uninstall.exe"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exported)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("export differs from installed uninstaller", err)
	}
	if err = compile(); err == nil {
		t.Fatal("existing export silently overwritten")
	}
	after, err := os.ReadFile(exported)
	if err != nil || !bytes.Equal(after, got) {
		t.Fatal("existing export changed", err)
	}
}

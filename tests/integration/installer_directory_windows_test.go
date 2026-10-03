//go:build windows

package integration

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNSISDirectoryValidation(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing", "empty", "nonempty", "file", "denied"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			if scenario == "file" {
				err = os.WriteFile(target, []byte("keep"), 0600)
			} else if scenario != "missing" {
				err = os.Mkdir(target, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "nonempty" {
				if err = os.WriteFile(filepath.Join(target, "keep.txt"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "denied" {
				user, err := windows.GetCurrentProcessToken().GetTokenUser()
				if err != nil {
					t.Fatal(err)
				}
				sid := user.User.Sid.String()
				set := func(sddl string) error {
					sd, err := windows.SecurityDescriptorFromString(sddl)
					if err != nil {
						return err
					}
					acl, _, err := sd.DACL()
					if err != nil {
						return err
					}
					return windows.SetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
				}
				if err = set("D:P(D;;0x1;;;" + sid + ")(A;;FA;;;" + sid + ")"); err != nil {
					t.Skipf("ACL fixture unavailable: %v", err)
				}
				t.Cleanup(func() {
					if err := set("D:P(A;;FA;;;" + sid + ")"); err != nil {
						t.Error(err)
					}
				})
			}
			script := fmt.Sprintf(`Unicode true
Name "Directory check"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
InstallDir "%s"
!include "%s"
Section
Call WinOneDirectoryIsEmpty
IfErrors rejected
SetErrorLevel 0
Quit
rejected:
SetErrorLevel 1
Quit
SectionEnd
`, filepath.Join(dir, "check.exe"), target, filepath.Join(root, "build", "windows", "directory.nsh"))
			path := filepath.Join(dir, "check.nsi")
			if err = os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v %s", err, output)
			}
			err = exec.Command(filepath.Join(dir, "check.exe")).Run()
			if scenario == "missing" || scenario == "empty" {
				if err != nil {
					t.Fatal(err)
				}
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("unsafe target accepted: %v", err)
			}
		})
	}
}

package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNSISCompilerAcceptsLiteralDollarPaths(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE for the real compiler check")
	}
	root := t.TempDir()
	payload := filepath.Join(root, "payload $ source")
	if err := os.MkdirAll(filepath.Join(payload, "licenses"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"WinOne.exe", "licenses/a$test.txt", "manifest.json"} {
		if err := os.WriteFile(filepath.Join(payload, filepath.FromSlash(name)), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := newManifest("0.1.0-alpha.1")
	manifest.Files = []File{{Path: "WinOne.exe"}, {Path: "licenses/a$test.txt"}}
	macros, err := NSISFileMacros(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "installed $ output")
	script := "Unicode true\nName \"WinOne file test\"\nOutFile \"" + filepath.Join(root, "check.exe") + "\"\nRequestExecutionLevel user\nInstallDir \"" + strings.ReplaceAll(target, "$", "$$") + "\"\n!define WINONE_PAYLOAD_DIR \"" + payload + "\"\n" + macros + "\nFunction Guard\nClearErrors\nFunctionEnd\nSection\n!insertmacro WinOneInstallFiles Guard failed\nGoto done\nfailed:\nSetErrorLevel 7\ndone:\nSectionEnd\n"
	path := filepath.Join(root, "check.nsi")
	if err = os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput()
	if err != nil {
		t.Fatalf("NSIS compile: %v\n%s", err, output)
	}
	if output, err = exec.Command(filepath.Join(root, "check.exe"), "/S").CombinedOutput(); err != nil {
		t.Fatalf("fixture install: %v %s", err, output)
	}
	if data, err := os.ReadFile(filepath.Join(target, "licenses", "a$test.txt")); err != nil || string(data) != "fixture" {
		t.Fatal("literal dollar filename did not install", err)
	}
	if err = os.WriteFile(filepath.Join(target, "user-note.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanup := strings.ReplaceAll(script, filepath.Join(root, "check.exe"), filepath.Join(root, "cleanup.exe"))
	cleanup = strings.ReplaceAll(cleanup, "!insertmacro WinOneInstallFiles Guard failed", "!insertmacro WinOneRemoveFiles Guard failed\n!insertmacro WinOneRemoveEmptyDirectories Guard failed")
	cleanupPath := filepath.Join(root, "cleanup.nsi")
	if err = os.WriteFile(cleanupPath, []byte(cleanup), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err = exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", cleanupPath).CombinedOutput(); err != nil {
		t.Fatalf("cleanup compile: %v %s", err, output)
	}
	if output, err = exec.Command(filepath.Join(root, "cleanup.exe"), "/S").CombinedOutput(); err != nil {
		t.Fatalf("fixture cleanup: %v %s", err, output)
	}
	if _, err = os.Stat(filepath.Join(target, "WinOne.exe")); !os.IsNotExist(err) {
		t.Fatal("known file was not removed")
	}
	if data, err := os.ReadFile(filepath.Join(target, "user-note.txt")); err != nil || string(data) != "keep" {
		t.Fatal("unlisted user file was removed", err)
	}
	if output, err = exec.Command(filepath.Join(root, "cleanup.exe"), "/S").CombinedOutput(); err != nil {
		t.Fatalf("repeated cleanup: %v %s", err, output)
	}
}

func TestNSISFileMacrosUseExactPathsAndPreserveUnknownFiles(t *testing.T) {
	manifest := newManifest("0.1.0-alpha.1")
	manifest.Files = []File{{Path: "WinOne.exe"}, {Path: "capture/zh-CN/sample.resources.dll"}, {Path: "licenses/a$test.txt"}}
	text, err := NSISFileMacros(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "RMDir /r") || strings.Contains(text, `File /r`) {
		t.Fatal("recursive file operations generated")
	}
	if !strings.Contains(text, `File "${WINONE_PAYLOAD_DIR}\licenses\a$test.txt"`) || !strings.Contains(text, `Delete "$INSTDIR\licenses\a$$test.txt"`) {
		t.Fatal("NSIS variable characters were not escaped")
	}
	if !strings.Contains(text, `Delete "$INSTDIR\manifest.json"`) {
		t.Fatal("installed manifest omitted")
	}
	child := strings.Index(text, `RMDir "$INSTDIR\capture\zh-CN"`)
	parent := strings.Index(text, `RMDir "$INSTDIR\capture"`)
	if child < 0 || parent < child {
		t.Fatal("directories were not removed child-first")
	}
}

func TestNSISFileMacrosRejectUnsafeAndDuplicatePaths(t *testing.T) {
	for _, paths := range [][]string{{"../file.dll"}, {"capture/CON.dll"}, {"WinOne.exe", "winone.exe"}, {"manifest.json"}} {
		manifest := newManifest("0.1.0-alpha.1")
		for _, name := range paths {
			manifest.Files = append(manifest.Files, File{Path: name})
		}
		if _, err := NSISFileMacros(manifest); err == nil {
			t.Fatalf("accepted unsafe inventory %v", paths)
		}
	}
}

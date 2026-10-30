//go:build windows

package distribution

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCurrentInstallerInterruptedRetry(t *testing.T) {
	if input := os.Getenv("WINONE_REPAIR_CHILD_REQUEST"); input != "" {
		// Exit without defers, as when Windows/installer is terminated after
		// writing files. Only the per-test process and isolated paths are used.
		raw, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		var r InstallerUpdate
		if err = json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		r.CheckStartup = func() error { os.Exit(77); return nil }
		if err = InstallCurrentPackage(r); err != nil {
			t.Fatal(err)
		}
		t.Fatal("fixture did not interrupt the installer")
	}
	r, update := repairFixture(t)
	// Function fields cannot enter JSON; the child uses the same real file
	// coordinator and replaces only the bounded startup check with termination.
	request := map[string]string{"Installation": update.Installation, "Profile": update.Profile, "Staged": update.Staged, "RegistryKey": update.RegistryKey, "ShortcutPath": update.ShortcutPath}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "request.json")
	if err = os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCurrentInstallerInterruptedRetry$")
	cmd.Env = append(os.Environ(), "WINONE_REPAIR_CHILD_REQUEST="+input)
	output, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 77 {
		t.Fatalf("child did not reach write completion: %v %s", err, output)
	}
	if _, err = os.Stat(filepath.Join(r.Installation, ".winone-pending")); err != nil {
		t.Fatal("interruption lost pending marker", err)
	}
	if _, err = VerifyInstalled(r.Installation, r.NextPackage); err == nil {
		t.Fatal("partial installation appeared complete")
	}
	// Repair with another payload, not the interrupted child's next package.
	update.Staged = extractTestPackage(t, r.PreviousPackage)
	uninstallerFixture(t, update.Staged, "latest repair uninstall")
	if err = os.WriteFile(filepath.Join(update.Staged, "WinOne.lnk"), r.NextShortcut.Data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = InstallCurrentPackage(update); err != nil {
		t.Fatal("new installer could not replace interrupted attempt", err)
	}
	if _, err = VerifyInstalled(r.Installation, r.PreviousPackage); err != nil {
		t.Fatal(err)
	}
	attempts, err := filepath.Glob(r.Installation + ".install-*")
	if err != nil || len(attempts) != 2 {
		t.Fatal("both attempts must retain their backups", attempts, err)
	}
}

func TestCurrentInstallerRejectsDirectoryInsteadOfProgram(t *testing.T) {
	r, update := repairFixture(t)
	program := filepath.Join(r.Installation, "WinOne.exe")
	if err := os.Rename(program, filepath.Join(t.TempDir(), "retained.exe")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(program, 0700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(program, "keep.txt")
	if err := os.WriteFile(keep, []byte("user directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := InstallCurrentPackage(update); err == nil {
		t.Fatal("non-file collision ignored")
	}
	raw, err := os.ReadFile(keep)
	if err != nil || string(raw) != "user directory" {
		t.Fatal("unrelated directory changed", err)
	}
}

func TestCurrentInstallerNeverTraversesPayloadJunction(t *testing.T) {
	r, update := repairFixture(t)
	external := t.TempDir()
	old := filepath.Join(r.Installation, "capture")
	if err := os.Rename(old, filepath.Join(external, "capture")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", old, filepath.Join(external, "capture"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated junction: %v %s", err, out)
	}
	if err := InstallCurrentPackage(update); err == nil {
		t.Fatal("repair followed an external junction")
	}
	// Verify external bytes against the unchanged old package's subset.
	manifest, err := Verify(r.PreviousPackage)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openUpgradeRoot(external)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, file := range manifest.Files {
		if len(file.Path) > 8 && file.Path[:8] == "capture/" {
			hash, err := upgradeFileHash(root, file.Path)
			if err != nil || hash != file.SHA256 {
				t.Fatal("external payload changed", file.Path, err)
			}
		}
	}
}

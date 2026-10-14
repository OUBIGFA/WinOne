//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRuntimeFilesKeepVerifiedBytesAndPathsStable(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	lease, err := holdRuntimeFiles(context.Background(), stage.Directory(), stage.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"bin/worker.exe", "LICENSE.txt", "module.json", "module.json.minisig", StageReceiptName} {
		path := filepath.Join(stage.Directory(), filepath.FromSlash(name))
		writer, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			writer.Close()
			t.Errorf("runtime file remained writable: %s", name)
		}
	}
	if err = os.Rename(stage.Directory(), stage.Directory()+"-moved"); err == nil {
		t.Fatal("runtime stage could be renamed")
	}
	if err = os.Rename(registry.directory, registry.directory+"-moved"); err == nil {
		t.Fatal("runtime store could be renamed")
	}
	publisher, err := options.Trust("example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.Resolve(context.Background(), "example.tool", publisher, options.Environment); err != nil {
		t.Fatal("file protection prevented verification", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(filepath.Join(stage.Directory(), "LICENSE.txt"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal("file protection survived close", err)
	}
	writer.Close()
	if err = os.Rename(stage.Directory(), stage.Directory()+"-moved"); err != nil {
		t.Fatal("directory protection survived close", err)
	}
}

func TestRuntimeFilesRejectExistingWriterAndReleasePartialLocks(t *testing.T) {
	_, stage, _ := runtimeFixture(t)
	writer, err := os.OpenFile(filepath.Join(stage.Directory(), "LICENSE.txt"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	lease, lockErr := holdRuntimeFiles(context.Background(), stage.Directory(), stage.Manifest())
	if lease != nil {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	}
	writer.Close()
	if lockErr == nil {
		t.Fatal("existing writer was accepted during execution verification")
	}
	if err = os.Rename(stage.Directory(), stage.Directory()+"-released"); err != nil {
		t.Fatal("failed acquisition leaked directory locks", err)
	}
}

func TestRuntimeFilesRejectLinkedAncestors(t *testing.T) {
	registry, stage, _ := runtimeFixture(t)
	link := filepath.Join(t.TempDir(), "store-alias")
	if err := os.Symlink(registry.directory, link); err != nil {
		// Junction creation is available to an ordinary Windows user. Pass the
		// fixture paths through environment variables, never shell interpolation.
		command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path $env:WINONE_TEST_JUNCTION_LINK -Target $env:WINONE_TEST_JUNCTION_TARGET | Out-Null`)
		command.Env = append(os.Environ(), "WINONE_TEST_JUNCTION_LINK="+link, "WINONE_TEST_JUNCTION_TARGET="+registry.directory)
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		if output, junctionErr := command.CombinedOutput(); junctionErr != nil {
			t.Skipf("directory links unavailable: %v; %v: %s", err, junctionErr, output)
		}
	}
	lease, err := holdRuntimeFiles(context.Background(), filepath.Join(link, filepath.Base(stage.Directory())), stage.Manifest())
	if lease != nil {
		if closeErr := lease.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("linked ancestor was accepted for runtime execution")
	}
}

func TestRuntimeFilesCancellationAndLongPaths(t *testing.T) {
	source, options := stageFixture(t)
	options.Directory = filepath.Join(t.TempDir(), strings.Repeat("a", 80), strings.Repeat("b", 80))
	if err := os.MkdirAll(options.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(filepath.Join(stage.Directory(), "bin", "worker.exe")) <= 260 {
		t.Fatal("fixture did not cover a long path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, err := holdRuntimeFiles(ctx, stage.Directory(), stage.Manifest()); !errors.Is(err, context.Canceled) || lease != nil {
		if lease != nil {
			lease.Close()
		}
		t.Fatal("cancelled acquisition retained locks", err)
	}
	lease, err := holdRuntimeFiles(context.Background(), stage.Directory(), stage.Manifest())
	if err != nil {
		t.Fatal("long-path protection failed", err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(stage.Directory(), stage.Directory()+"-closed"); err != nil {
		t.Fatal("long-path protection was not released", err)
	}
}

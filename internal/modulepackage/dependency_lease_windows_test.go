//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func dependencyLeaseFixture(t *testing.T) (*DependencySource, *VerifiedManifest) {
	t.Helper()
	ctx := context.Background()
	directory := t.TempDir()
	modules, err := OpenRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := OpenRuntimeStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	base, publisher, key := signedTestManifest(t)
	runtimeManifest := base
	runtimeManifest.Format = "winone.runtime.v1"
	runtimeManifest.Metadata = runtimeMetadataFixture()
	file := writeRuntimeArchive(t, runtimeManifest, publisher, key)
	checked, err := VerifyArchive(ctx, file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	ref := graphReference(checked.Manifest())
	if _, err = runtimes.Install(ctx, file, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: ref.ManifestSHA256}); err != nil {
		t.Fatal(err)
	}
	source, err := NewDependencySource(modules, runtimes, testEnvironment(), func(string) (Publisher, error) { return publisher, nil })
	if err != nil {
		t.Fatal(err)
	}
	moduleFile := dependencyModuleArchive(t, "example.consumer", []Dependency{ref}, base, publisher, key)
	module, err := VerifyArchive(ctx, moduleFile, publisher)
	if err != nil {
		t.Fatal(err)
	}
	return source, module.Manifest()
}

func TestDependencyContextRejectsLinkedDataAncestorsBeforeWriting(t *testing.T) {
	source, manifest := dependencyLeaseFixture(t)
	lease, err := source.Hold(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "data-alias")
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; New-Item -ItemType Junction -Path $env:WINONE_TEST_LINK -Target $env:WINONE_TEST_TARGET | Out-Null`)
	command.Env = append(os.Environ(), "WINONE_TEST_LINK="+link, "WINONE_TEST_TARGET="+target)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create isolated directory link: %v %s", err, output)
	}
	if _, err = lease.WriteContext(filepath.Join(link, "new-worker"), `\\.\pipe\WinOne-dependency-host`); err == nil {
		t.Fatal("dependency context accepted a linked data ancestor")
	}
	if _, err = os.Stat(filepath.Join(target, "new-worker")); !os.IsNotExist(err) {
		t.Fatal("rejected context wrote through the directory link", err)
	}
}

func TestDependencyLeasePinsRuntimeAndContextUntilClosed(t *testing.T) {
	ctx := context.Background()
	source, manifest := dependencyLeaseFixture(t)
	lease, err := source.Hold(ctx, manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	contextPath, err := lease.WriteContext(t.TempDir(), `\\.\pipe\WinOne-dependency-host`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	var view DependencyContext
	if err = json.Unmarshal(raw, &view); err != nil || len(view.Packages) != 1 || view.Root.ID != "example.consumer" {
		t.Fatal(view, err)
	}
	if len(view.Packages[0].Exports) != 2 || !filepath.IsAbs(view.Packages[0].Exports[1].Path) {
		t.Fatal("runtime paths were not resolved", view)
	}
	if err = os.WriteFile(view.Packages[0].Exports[1].Path, []byte("changed"), 0600); err == nil {
		t.Fatal("active dependency was writable")
	}
	if err = os.WriteFile(contextPath, []byte("{}"), 0600); err == nil {
		t.Fatal("active dependency context was writable")
	}
	if err = lease.Recheck(ctx); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(contextPath, []byte("{}"), 0600); err != nil {
		t.Fatal("context handle was not released", err)
	}
}

func TestDependencyLeaseContextCannotWriteInsidePackageStore(t *testing.T) {
	source, manifest := dependencyLeaseFixture(t)
	lease, err := source.Hold(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	for _, directory := range []string{source.modules.directory, filepath.Join(source.modules.directory, "new-data")} {
		if _, err := lease.WriteContext(directory, `\\.\pipe\WinOne-dependency-host`); err == nil {
			t.Fatal("worker context was written inside the package store")
		}
		if _, err := os.Stat(filepath.Join(directory, "dependencies.json")); !os.IsNotExist(err) {
			t.Fatal("rejected context changed package contents", err)
		}
	}
}

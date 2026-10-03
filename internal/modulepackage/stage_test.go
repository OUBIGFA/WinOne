package modulepackage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stageFixture(t *testing.T) (string, StageOptions) {
	t.Helper()
	data, publisher := archiveFixture(t, nil, false)
	source := filepath.Join(t.TempDir(), "tool.winone-module")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyArchive(context.Background(), source, publisher)
	if err != nil {
		t.Fatal(err)
	}
	return source, StageOptions{Directory: t.TempDir(), Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()}
}
func TestStagingCopiesVerifiedPayloadAndWritesReceiptLast(t *testing.T) {
	source, options := stageFixture(t)
	stage, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	if !stage.Ready() {
		t.Fatal("successful stage is not ready")
	}
	worker, err := os.ReadFile(filepath.Join(stage.Directory(), "bin", "worker.exe"))
	if err != nil || string(worker) != "inert worker" {
		t.Fatal("wrong staged payload", err)
	}
	raw, err := os.ReadFile(filepath.Join(stage.Directory(), StageReceiptName))
	if err != nil {
		t.Fatal(err)
	}
	var receipt StageReceipt
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ManifestSHA256 != options.ExpectedManifestSHA256 || receipt.ModuleID != "example.tool" || receipt.Version != "1.2.0" {
		t.Fatal(receipt)
	}
	if _, err = os.Stat(filepath.Join(stage.Directory(), "module.json.minisig")); err != nil {
		t.Fatal(err)
	}
}
func TestChangedApprovalAndIncompatibleHostCreateNoStage(t *testing.T) {
	for _, change := range []func(*StageOptions){func(o *StageOptions) { o.ExpectedManifestSHA256 = "" }, func(o *StageOptions) { o.ExpectedManifestSHA256 = strings.Repeat("0", 64) }, func(o *StageOptions) { o.Environment.ProtocolVersion = 2 }} {
		source, options := stageFixture(t)
		change(&options)
		if stage, err := StageArchive(context.Background(), source, options); err == nil || stage != nil {
			t.Fatal("unapproved stage created", stage, err)
		}
		files, err := os.ReadDir(options.Directory)
		if err != nil || len(files) != 0 {
			t.Fatal("rejected stage left files", err)
		}
	}
}
func TestStagingNeverMarksChangedPayloadReady(t *testing.T) {
	source, options := stageFixture(t)
	data, publisher := archiveFixture(t, func(files map[string][]byte) { files["bin/worker.exe"] = []byte("other worker") }, false)
	options.Publisher = publisher
	// Same signed manifest, newly signed fixture key, but changed payload bytes.
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	stage, err := StageArchive(context.Background(), source, options)
	if err == nil || stage == nil || stage.Ready() {
		t.Fatal("bad stage was marked ready", err)
	}
	if _, err = os.Stat(filepath.Join(stage.Directory(), StageReceiptName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed stage has ready receipt", err)
	}
}
func TestStageControlFilesDoNotOverwriteExistingContent(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.WriteFile("module.json", []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = writeStageFile(root, "module.json", []byte("replace")); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing stage file overwritten", err)
	}
	data, err := root.ReadFile("module.json")
	if err != nil || string(data) != "keep" {
		t.Fatal(err)
	}
}

func TestStagingRequiresAnExplicitAbsoluteStore(t *testing.T) {
	source, options := stageFixture(t)
	t.Chdir(t.TempDir())
	options.Directory = ""
	if stage, err := StageArchive(context.Background(), source, options); err == nil || stage != nil {
		t.Fatal("missing store silently used working directory")
	}
}

func TestStagedPayloadDoesNotOverwriteExistingFile(t *testing.T) {
	source, options := stageFixture(t)
	inspection, err := inspectArchive(context.Background(), source, options.Publisher)
	if err != nil {
		t.Fatal(err)
	}
	defer inspection.file.Close()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.Mkdir("bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err = root.WriteFile("bin/worker.exe", []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = inspection.copyPayload(context.Background(), root); !errors.Is(err, os.ErrExist) {
		t.Fatal("payload overwrote an existing file", err)
	}
	data, err := root.ReadFile("bin/worker.exe")
	if err != nil || string(data) != "keep" {
		t.Fatal(err)
	}
}

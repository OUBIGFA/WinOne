package modulepackage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagedDirectoryIsReverifiedAfterReopening(t *testing.T) {
	source, options := stageFixture(t)
	stage, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := VerifyStage(context.Background(), filepath.Base(stage.Directory()), options)
	if err != nil || !reopened.Ready() || reopened.Manifest().Digest() != options.ExpectedManifestSHA256 {
		t.Fatal("valid stage did not recover", err)
	}
}
func TestStagedReceiptCannotHidePayloadChanges(t *testing.T) {
	for _, change := range []func(string) error{
		func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "bin", "worker.exe"), []byte("other worker"), 0600)
		},
		func(dir string) error { return os.Remove(filepath.Join(dir, "LICENSE.txt")) },
		func(dir string) error { return os.WriteFile(filepath.Join(dir, "extra.dll"), []byte("unlisted"), 0600) },
		func(dir string) error { return os.Mkdir(filepath.Join(dir, "unlisted"), 0700) },
		func(dir string) error {
			raw, err := os.ReadFile(filepath.Join(dir, StageReceiptName))
			if err != nil {
				return err
			}
			var receipt StageReceipt
			if err = json.Unmarshal(raw, &receipt); err != nil {
				return err
			}
			receipt.ManifestSHA256 = strings.Repeat("0", 64)
			raw, err = json.Marshal(receipt)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, StageReceiptName), raw, 0600)
		},
	} {
		source, options := stageFixture(t)
		stage, err := StageArchive(context.Background(), source, options)
		if err != nil {
			t.Fatal(err)
		}
		if err = change(stage.Directory()); err != nil {
			t.Fatal(err)
		}
		if recovered, err := VerifyStage(context.Background(), filepath.Base(stage.Directory()), options); err == nil || recovered != nil {
			t.Fatal("modified stage recovered as ready")
		}
	}
}
func TestStageRecoveryRequiresApprovedIdentityAndKnownEnvironment(t *testing.T) {
	source, options := stageFixture(t)
	stage, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(stage.Directory())
	for _, invalid := range []string{"", "..", "../" + id, stage.Directory()} {
		if _, err = VerifyStage(context.Background(), invalid, options); err == nil {
			t.Fatal("unsafe stage identity accepted")
		}
	}
	options.ExpectedManifestSHA256 = strings.Repeat("0", 64)
	if _, err = VerifyStage(context.Background(), id, options); err == nil {
		t.Fatal("approval mismatch ignored")
	}
	options.ExpectedManifestSHA256 = stage.Manifest().Digest()
	options.Environment.HostVersion = "unknown"
	if _, err = VerifyStage(context.Background(), id, options); err == nil {
		t.Fatal("unknown environment accepted")
	}
}

//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyPreparationRecoveryAdoptsOnlyCompleteUnreferencedStages(t *testing.T) {
	source, options := stageFixture(t)
	registry, err := OpenRegistry(options.Directory)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	tracked, err := registry.StageArchive(context.Background(), source, options, "tracked-install")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := StageArchive(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.RegisterStage(context.Background(), filepath.Base(installed.Directory()), options, 0); err != nil {
		t.Fatal(err)
	}
	incomplete := "stage-00000000000000000000000000000000"
	if err = os.Mkdir(filepath.Join(options.Directory, incomplete), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(options.Directory, incomplete, "keep.txt"), []byte("unverified"), 0600); err != nil {
		t.Fatal(err)
	}
	ids, err := registry.UntrackedStages(context.Background())
	if err != nil || len(ids) != 2 {
		t.Fatal("untracked directory detection is incomplete", ids, err)
	}
	result, err := registry.RecoverPreparations(context.Background(), func(string) (Publisher, error) { return options.Publisher, nil })
	if err != nil || result.Recovered != 1 || len(result.Issues) != 1 || result.Issues[0].StageID != incomplete {
		t.Fatal("recovery adopted unknown content or missed a valid stage", result, err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || len(ledger.Items) != 2 {
		t.Fatal(ledger, err)
	}
	seen := map[string]bool{}
	for _, item := range ledger.Items {
		seen[item.Package.StageID] = true
	}
	if !seen[filepath.Base(ready.Directory())] || !seen[filepath.Base(tracked.Directory())] || seen[filepath.Base(installed.Directory())] {
		t.Fatal("recovery targeted the wrong stages", seen)
	}
	if raw, err := os.ReadFile(filepath.Join(options.Directory, incomplete, "keep.txt")); err != nil || string(raw) != "unverified" {
		t.Fatal("unknown files changed", err)
	}
	again, err := registry.RecoverPreparations(context.Background(), func(string) (Publisher, error) { return options.Publisher, nil })
	if err != nil || again.Recovered != 0 || len(again.Issues) != 1 {
		t.Fatal("repeat recovery duplicated work", again, err)
	}
}

func TestLegacyPreparationRecoveryPreservesChangedUntrustedAndCancelledStages(t *testing.T) {
	for _, mode := range []string{"changed", "untrusted", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			source, options := stageFixture(t)
			stage, err := StageArchive(context.Background(), source, options)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "changed" {
				if err = os.WriteFile(filepath.Join(stage.Directory(), "bin/worker.exe"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := OpenRegistry(options.Directory)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			result, err := registry.RecoverPreparations(ctx, func(string) (Publisher, error) {
				if mode == "untrusted" {
					return Publisher{}, ErrUntrustedPublisher
				}
				return options.Publisher, nil
			})
			if mode == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if err != nil || result.Recovered != 0 || len(result.Issues) != 1 {
				t.Fatal(result, err)
			}
			ledger, readErr := registry.Reclamation()
			if readErr != nil || len(ledger.Items) != 0 {
				t.Fatal("unsafe stage gained cleanup ownership", ledger, readErr)
			}
			if _, err = os.Stat(stage.Directory()); err != nil {
				t.Fatal("stage was removed", err)
			}
		})
	}
}

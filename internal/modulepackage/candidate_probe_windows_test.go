//go:build windows

package modulepackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/modulestate"
)

func TestCandidateProbeDistinguishesNoProcessFromMissingRestoreAcknowledgement(t *testing.T) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") != "1" {
		t.Skip("explicit isolated-process probe")
	}
	for _, mode := range []string{"valid", "invalid-image", "early-exit", "refuse-shutdown"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			_, _, options := runtimeFixture(t)
			registry, err := OpenRegistry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			options.State, err = modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			stage, approval := signedRuntimeStage(t, registry, "1.2.0", mode == "invalid-image")
			if _, err = registry.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, 0, options.State); err != nil {
				t.Fatal(err)
			}
			options.Trust = func(string) (Publisher, error) { return approval.Publisher, nil }
			runtime, err := registry.PrepareRuntime(ctx, "example.tool", options)
			if err != nil {
				t.Fatal(err)
			}
			marker := ""
			if mode == "early-exit" {
				marker = "exit-before-probe"
			} else if mode == "refuse-shutdown" {
				marker = "refuse-shutdown"
			}
			if marker != "" {
				if err = os.MkdirAll(options.DataDirectory, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(options.DataDirectory, marker), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if marker != "" {
					_ = os.Remove(filepath.Join(options.DataDirectory, marker))
				}
				if err := runtime.Close(); err != nil {
					t.Error(err)
				}
			})
			restored, err := runtime.ProbeCandidate()
			if mode == "valid" {
				if err != nil || !restored {
					t.Fatal("valid candidate was not checked and closed", restored, err)
				}
			} else if err == nil {
				t.Fatal("invalid candidate accepted")
			}
			if mode == "invalid-image" {
				if !restored || runtime.Running() {
					t.Fatal("a rejected executable was treated as an unacknowledged running process")
				}
			}
			if mode == "early-exit" || mode == "refuse-shutdown" {
				if restored {
					t.Fatal("unacknowledged process was treated as restored")
				}
			}
		})
	}
}

//go:build windows

package distribution

import (
	"errors"
	"os"
	"testing"
)

func TestInstallerProgressFollowsWorkAndRollback(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[broken], func(t *testing.T) {
			req := upgradeExecutionFixture(t, true)
			last := ""
			seen := map[string]bool{}
			checkStage := func(want string) {
				if last != want {
					t.Fatalf("work ran with stage %q; want %q", last, want)
				}
			}
			update := InstallerUpdate{Installation: req.Installation, Profile: req.Profile, Staged: req.Staged, RegistryKey: req.RegistryKey, ShortcutPath: req.ShortcutPath,
				Progress:           func(stage string) { last = stage; seen[stage] = true },
				CheckConfiguration: func() error { checkStage("checking-configuration"); return nil },
				Prepare:            func() error { checkStage("restoring-resources"); return nil },
				CheckStartup: func() error {
					checkStage("checking-startup")
					if broken {
						return errors.New("fixture startup failed")
					}
					return nil
				},
			}
			err := RunInstallerUpdate(update)
			if (err != nil) != broken {
				t.Fatal(err)
			}
			for _, stage := range []string{"verifying", "backing-up", "applying", "checking-configuration", "restoring-resources", "checking-startup"} {
				if !seen[stage] {
					t.Fatalf("missing real phase %s", stage)
				}
			}
			if seen["rolling-back"] != broken {
				t.Fatal("rollback progress did not match work")
			}
			if broken && last != "rolling-back" {
				t.Fatalf("failure reported a later completion phase: %s", last)
			}
		})
	}
}

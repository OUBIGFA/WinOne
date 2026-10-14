//go:build windows

package startup

import (
	"errors"
	"golang.org/x/sys/windows"
	"testing"
	"winone/modules/wintray/internal/config"
)

func TestReleaseAllNeedsAtMostOneElevationForSeveralTasks(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "approved", true: "cancelled"}[cancelled], func(t *testing.T) {
			fake := newFakeSchtasks()
			tasks := testAppTasks(t, fake)
			tasks.elevated = func() bool { return false }
			for _, id := range []string{"1", "2", "3"} {
				fake.tasks[tasks.namePrefix+id] = `<Task/>`
			}
			fake.tasks["AnotherOwner"] = `<Task/>`
			fake.deleteErr = errors.New("access denied")
			prompts := 0
			tasks.elevateSelf = func(args ...string) error {
				prompts++
				if args[0] != AppTaskHelperRelease {
					t.Fatalf("per-task elevation returned: %v", args)
				}
				if cancelled {
					return windows.ERROR_CANCELLED
				}
				fake.deleteErr = nil
				return tasks.Sync(config.Settings{}, false)
			}
			err := tasks.ReleaseAll()
			if cancelled {
				if !errors.Is(err, windows.ERROR_CANCELLED) || len(fake.tasks) != 4 {
					t.Fatalf("cancel lost tasks or error: %v %v", fake.tasks, err)
				}
			} else if err != nil || len(fake.tasks) != 1 || fake.tasks["AnotherOwner"] == "" {
				t.Fatalf("release failed: %v %v", fake.tasks, err)
			}
			if prompts != 1 {
				t.Fatalf("got %d authorization prompts", prompts)
			}
		})
	}
}

func TestReleaseAllDoesNotPromptWithSufficientPermission(t *testing.T) {
	fake := newFakeSchtasks()
	tasks := testAppTasks(t, fake)
	tasks.elevated = func() bool { return true }
	fake.tasks[tasks.namePrefix+"1"] = `<Task/>`
	tasks.elevateSelf = func(...string) error { t.Fatal("an already elevated release asked again"); return nil }
	if err := tasks.ReleaseAll(); err != nil || len(fake.tasks) != 0 {
		t.Fatal(err, fake.tasks)
	}
}

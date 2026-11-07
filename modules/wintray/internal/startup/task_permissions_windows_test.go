//go:build windows

package startup

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// A LeastPrivilege task can still have an administrator-owned ACL. Its run
// level must not prevent recovery after moving or updating WinOne.
func TestAppTasksRepairProtectedOrdinaryTask(t *testing.T) {
	for _, source := range []string{"configured", "migrated Run entry"} {
		t.Run(source, func(t *testing.T) {
			fake := newFakeSchtasks()
			tasks := testAppTasks(t, fake)
			tasks.elevated = func() bool { return false }
			settings := taskSettings(`C:\Apps\app.exe`)
			if source == "migrated Run entry" {
				_, run, _ := withRunSource(t, tasks)
				if err := run.SetStringValue("Example", `"C:\Apps\app.exe" --startup`); err != nil {
					t.Fatal(err)
				}
			}
			if err := tasks.Sync(settings, false); err != nil {
				t.Fatal(err)
			}
			name := tasks.namePrefix + settings.ManagedApps[0].ID
			original := fake.tasks[name]
			tasks.selfExe = `C:\Moved\WinOne.WinTray.exe`
			fake.createErr = windows.ERROR_ACCESS_DENIED
			fake.deleteErr = windows.ERROR_ACCESS_DENIED
			prompts := 0
			tasks.elevateSelf = func(args ...string) error {
				prompts++
				if len(args) != 3 || args[0] != AppTaskHelperRegister || args[1] != name {
					t.Fatalf("unexpected repair: %v", args)
				}
				definition, err := readTaskDefinition(args[2])
				if err == nil {
					fake.tasks[args[1]] = definition
				}
				return err
			}
			if err := tasks.Sync(settings, false); !errors.Is(err, errElevationNeeded) || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				t.Fatalf("background repair = %v; want pending elevation and original error", err)
			}
			if prompts != 0 || fake.tasks[name] != original {
				t.Fatal("background repair prompted or changed the protected task")
			}
			if err := tasks.Sync(settings, true); err != nil {
				t.Fatalf("confirmed repair: %v", err)
			}
			var repaired taskDefinition
			if err := decodeTaskXML([]byte(fake.tasks[name]), &repaired); err != nil {
				t.Fatal(err)
			}
			if len(repaired.Actions.Exec) != 1 || repaired.Actions.Exec[0].Command != tasks.selfExe || repaired.Principals[0].RunLevel != "LeastPrivilege" {
				t.Fatalf("repair changed the program's privilege or missed the new path: %+v", repaired)
			}
			if err := tasks.Sync(settings, true); err != nil || prompts != 1 {
				t.Fatalf("unchanged repaired task prompted again: prompts=%d err=%v", prompts, err)
			}
		})
	}
}

func TestTaskRegistrationPermissionFailures(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, tc := range []struct {
			name                     string
			highest, elevated, allow bool
			createErr, helperErr     error
			wantPrompt, wantPending  bool
		}{
			{name: "ordinary success"},
			{name: "protected background", createErr: windows.ERROR_ACCESS_DENIED, wantPending: true},
			{name: "protected confirmed", allow: true, createErr: windows.ERROR_ACCESS_DENIED, wantPrompt: true},
			{name: "confirmation canceled", allow: true, createErr: windows.ERROR_ACCESS_DENIED, helperErr: windows.ERROR_CANCELLED, wantPrompt: true},
			{name: "administrator failure", elevated: true, allow: true, createErr: windows.ERROR_ACCESS_DENIED},
			{name: "highest background", highest: true, wantPending: true},
			{name: "highest confirmed", highest: true, allow: true, wantPrompt: true},
			{name: "already administrator", highest: true, elevated: true},
		} {
			kind := "replacement/"
			if native {
				kind = "native/"
			}
			t.Run(kind+tc.name, func(t *testing.T) {
				fake := newFakeSchtasks()
				tasks := testAppTasks(t, fake)
				tasks.elevated = func() bool { return tc.elevated }
				fake.createErr = tc.createErr
				spec := appTaskSpec{name: tasks.namePrefix + "test", exePath: `C:\Apps\app.exe`, highest: tc.highest}
				definition, _ := appTaskXML(tasks.userSID, spec)
				prompts := 0
				tasks.elevateSelf = func(args ...string) error {
					prompts++
					if len(args) != 3 || args[0] != AppTaskHelperRegister || args[1] != spec.name {
						t.Fatalf("unexpected helper arguments: %v", args)
					}
					if tc.helperErr != nil {
						return tc.helperErr
					}
					data, err := readTaskDefinition(args[2])
					if err == nil {
						fake.tasks[spec.name] = data
					}
					return err
				}
				var err error
				if native {
					err = tasks.writeNativeTask(spec.name, definition, tc.allow)
				} else {
					err = tasks.syncOne(spec, tc.allow)
				}
				if (prompts == 1) != tc.wantPrompt || prompts > 1 {
					t.Fatalf("prompts=%d wantPrompt=%v", prompts, tc.wantPrompt)
				}
				if errors.Is(err, errElevationNeeded) != tc.wantPending {
					t.Fatalf("pending elevation: %v", err)
				}
				wantErr := tc.wantPending || tc.helperErr != nil || (tc.elevated && tc.createErr != nil)
				if (err != nil) != wantErr {
					t.Fatalf("error=%v wantErr=%v", err, wantErr)
				}
				if tc.helperErr != nil && !errors.Is(err, tc.helperErr) {
					t.Fatalf("lost helper failure: %v", err)
				}
				if tc.createErr != nil && wantErr && !errors.Is(err, tc.createErr) {
					t.Fatalf("lost original failure: %v", err)
				}
				if wantErr && fake.tasks[spec.name] != "" {
					t.Fatal("failed operation was accepted")
				}
				if !wantErr && fake.tasks[spec.name] == "" {
					t.Fatal("successful operation did not register the task")
				}
			})
		}
	}
}

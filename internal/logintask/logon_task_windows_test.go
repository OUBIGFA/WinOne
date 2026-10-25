//go:build windows

package logintask

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProductLoginIsImmediateInteractiveAndLeastPrivilege(t *testing.T) {
	definition, _ := logonTaskXML("S-1-5-21-1", `C:\Apps\WinOne.Host.exe`, "--autorun")
	for _, value := range []string{"<LogonTrigger>", "<UserId>S-1-5-21-1</UserId>", "<LogonType>InteractiveToken</LogonType>", "<RunLevel>LeastPrivilege</RunLevel>", "<Arguments>--autorun</Arguments>"} {
		if !strings.Contains(definition, value) {
			t.Fatal(value)
		}
	}
	if strings.Contains(definition, "<Delay>") || strings.Contains(definition, "HighestAvailable") {
		t.Fatal("login task delays or elevates")
	}
}

// Explicit opt-in: create only this test's uniquely named task, never run it,
// verify its definition, then remove it. This does not change WinOne's task.
func TestLoginTaskRegistrationAsStandardToken(t *testing.T) {
	if os.Getenv("WINONE_TEST_LOGIN_TASKS") != "1" {
		t.Skip("set WINONE_TEST_LOGIN_TASKS=1 for isolated scheduler verification")
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires a non-elevated token")
	}
	task, err := NewProductLogonTask()
	if err != nil {
		t.Fatal(err)
	}
	task.name = fmt.Sprintf("WinOne-LoginProbe-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		if err := task.Remove(); err != nil {
			t.Error(err)
		}
	})
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if err = task.Sync(exe, "/c exit"); err != nil {
		t.Fatal(err)
	}
	if err = task.Sync(exe, "/c exit"); err != nil {
		t.Fatal(err)
	}
	exported, err := task.run("/Query", "/TN", task.name, "/XML")
	if err != nil {
		t.Fatal(err)
	}
	_, fingerprint := logonTaskXML(task.userSID, exe, "/c exit")
	if !taskUpToDate(exported, fingerprint) {
		t.Fatal("registered task does not match the requested definition")
	}
	if _, err = runAdministratorTask(task.name, InteractiveLaunch, task.userSID, exe); err == nil {
		t.Fatal("ordinary task was accepted as an authorized launcher")
	}
	if err = task.Remove(); err != nil {
		t.Fatal(err)
	}
}
func TestLoginTaskQueryFailureDoesNotRegister(t *testing.T) {
	denied := errors.New("task query denied")
	for _, check := range []struct {
		name   string
		exists bool
		err    error
	}{
		{"existing", true, nil},
		{"lookup denied", false, denied},
		{"inconsistent lookup", true, denied},
	} {
		t.Run(check.name, func(t *testing.T) {
			var calls []string
			task := &LogonTask{name: "test", userSID: "S-1-5-21-1", run: func(args ...string) ([]byte, error) {
				calls = append(calls, args[0])
				return nil, denied
			}, lookup: func(string) (bool, error) { return check.exists, check.err }}
			if err := task.Sync(`C:\WinOne\WinOne.Host.exe`, "--autorun"); !errors.Is(err, denied) {
				t.Fatal("query failure was not preserved", err)
			}
			if strings.Join(calls, ",") != "/Query" {
				t.Fatal("uncertain task state caused a registration attempt", calls)
			}
		})
	}
}

func TestLoginTaskCreatesOnlyAfterConfirmedAbsenceWithoutForce(t *testing.T) {
	queried, lookedUp, created := false, false, false
	task := &LogonTask{name: "test", userSID: "S-1-5-21-1", lookup: func(string) (bool, error) { lookedUp = true; return false, nil }}
	task.run = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "/Query":
			queried = true
			return nil, errors.New("not found")
		case "/Create":
			if !queried || !lookedUp {
				t.Fatal("creation preceded absence verification")
			}
			for _, arg := range args {
				if arg == "/F" {
					t.Fatal("new registration can overwrite a concurrently created task")
				}
			}
			created = true
			return nil, nil
		default:
			t.Fatal(args)
			return nil, nil
		}
	}
	if err := task.Sync(`C:\WinOne\WinOne.Host.exe`, "--autorun"); err != nil || !created {
		t.Fatal(created, err)
	}
}

func TestLoginTaskUnchangedDefinitionDoesNotWrite(t *testing.T) {
	const executable = `C:\WinOne\WinOne.Host.exe`
	definition, _ := logonTaskXML("S-1-5-21-1", executable, "--autorun")
	calls := 0
	task := &LogonTask{name: "test", userSID: "S-1-5-21-1", run: func(args ...string) ([]byte, error) {
		calls++
		if args[0] != "/Query" {
			t.Fatal("unchanged startup issued a write", args)
		}
		return utf16LEWithBOM(definition), nil
	}}
	if err := task.Sync(executable, "--autorun"); err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
}

func TestUnknownProductTaskDoesNotIssueDelete(t *testing.T) {
	denied := errors.New("lookup denied")
	task := &LogonTask{name: "test", lookup: func(string) (bool, error) { return false, denied }, run: func(...string) ([]byte, error) { t.Fatal("deletion attempted after failed lookup"); return nil, nil }}
	if err := task.Remove(); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	task.lookup = nil
	if err := task.Remove(); err == nil {
		t.Fatal("missing lookup accepted")
	}
}

func TestAbsentProductTaskDoesNotIssueDelete(t *testing.T) {
	called := false
	task := &LogonTask{name: "test", run: func(...string) ([]byte, error) { called = true; return nil, nil }, lookup: func(string) (bool, error) { return false, nil }}
	if err := task.Remove(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("disabled startup issued a system deletion for an absent task")
	}
}

func TestProductTaskRemovalDoesNotHideDeniedAccess(t *testing.T) {
	denied := errors.New("denied")
	task := &LogonTask{name: "test", run: func(...string) ([]byte, error) { return nil, denied }, lookup: func(string) (bool, error) { return true, nil }}
	if !errors.Is(task.Remove(), denied) {
		t.Fatal("denied removal hidden")
	}
	task.lookup = func(string) (bool, error) { return false, nil }
	if err := task.Remove(); err != nil {
		t.Fatal(err)
	}
}

func TestAdministratorTaskDefinitionIsSeparateAndOnDemand(t *testing.T) {
	normal, normalID := logonTaskXML("S-1-5-21-1", `C:\WinOne\WinOne.exe`, "--open-settings")
	elevated, elevatedID := taskXML("S-1-5-21-1", `C:\WinOne\WinOne.exe`, "--open-settings", true, true)
	if normalID == elevatedID {
		t.Fatal("authorization level missing from task identity")
	}
	if !strings.Contains(normal, "LeastPrivilege") || !strings.Contains(normal, "<LogonTrigger>") {
		t.Fatal("ordinary login definition changed")
	}
	if !strings.Contains(elevated, "HighestAvailable") || strings.Contains(elevated, "<LogonTrigger>") {
		t.Fatal("authorized launcher is not on-demand")
	}
}

func TestAdministratorTaskCannotBeRegisteredWithoutConsent(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires a non-elevated token")
	}
	called := false
	task := &LogonTask{elevated: true, run: func(...string) ([]byte, error) { called = true; return nil, nil }}
	if err := task.Sync(`C:\WinOne\WinOne.exe`, "--open-settings"); err == nil {
		t.Fatal("missing authorization accepted")
	}
	if called {
		t.Fatal("scheduler touched before consent")
	}
}

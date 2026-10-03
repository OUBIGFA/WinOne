//go:build windows

package logintask

import (
	"strings"
	"testing"
)

func TestAdministratorDefinitionRejectsChangedActionsAndPrincipals(t *testing.T) {
	const sid = "S-1-5-21-1"
	const exe = `C:\WinOne\WinOne.exe`
	definition, _ := taskXML(sid, exe, administratorLaunchArguments, true, true)
	if err := validateAdministratorDefinition([]byte(definition), sid, exe); err != nil {
		t.Fatal(err)
	}
	if err := validateAdministratorDefinition(utf16LEWithBOM(definition), sid, exe); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"other executable":       strings.Replace(definition, exe, `C:\Other\WinOne.exe`, 1),
		"different arguments":    strings.Replace(definition, "--authorized-start", "--other", 1),
		"other user":             strings.ReplaceAll(definition, sid, "S-1-5-21-2"),
		"ordinary token":         strings.Replace(definition, "HighestAvailable", "LeastPrivilege", 1),
		"extra action":           strings.Replace(definition, "</Actions>", "<Exec><Command>other.exe</Command></Exec></Actions>", 1),
		"changed action type":    strings.Replace(definition, "</Actions>", "<ComHandler><ClassId>other</ClassId></ComHandler></Actions>", 1),
		"login trigger":          strings.Replace(definition, "<Triggers />", "<Triggers><LogonTrigger /></Triggers>", 1),
		"disabled":               strings.Replace(definition, "<Enabled>true</Enabled>", "<Enabled>false</Enabled>", 1),
		"cannot start on demand": strings.Replace(definition, "<Settings>", "<Settings><AllowStartOnDemand>false</AllowStartOnDemand>", 1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateAdministratorDefinition([]byte(value), sid, exe); err == nil {
				t.Fatal("changed task accepted")
			}
		})
	}
}

func TestAdministratorDefinitionAcceptsOmittedEnabledDefault(t *testing.T) {
	const sid = "S-1-5-21-1"
	const exe = `C:\WinOne\WinOne.exe`
	definition, _ := taskXML(sid, exe, administratorLaunchArguments, true, true)
	// Windows omits Enabled=true when exporting the registered task. Absence
	// uses the schema default; it must not invalidate a successful authorization.
	exported := strings.Replace(definition, "<Enabled>true</Enabled>", "", 1)
	if exported == definition {
		t.Fatal("fixture did not omit Enabled")
	}
	if err := validateAdministratorDefinition([]byte(exported), sid, exe); err != nil {
		t.Fatal(err)
	}
}

func TestAdministratorRegistrationDoesNotReuseChangedTask(t *testing.T) {
	const sid = "S-1-5-21-1"
	const exe = `C:\WinOne\WinOne.exe`
	definition, fingerprint := taskXML(sid, exe, administratorLaunchArguments, true, true)
	task := &LogonTask{userSID: sid, elevated: true, onDemand: true}
	for name, exported := range map[string]string{
		"unchanged":          definition,
		"changed action":     strings.Replace(definition, "--authorized-start", "--other-action", 1),
		"changed user":       strings.ReplaceAll(definition, sid, "S-1-5-21-2"),
		"changed permission": strings.Replace(definition, "HighestAvailable", "LeastPrivilege", 1),
	} {
		t.Run(name, func(t *testing.T) {
			// A description is not evidence that the current action still matches.
			if !strings.Contains(exported, fingerprint) {
				t.Fatal("fixture changed the description")
			}
			if got := task.canReuse([]byte(exported), fingerprint, exe); got != (name == "unchanged") {
				t.Fatalf("canReuse=%v", got)
			}
		})
	}
}

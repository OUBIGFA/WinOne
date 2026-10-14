//go:build windows

package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConsoleLaunchPreservesHandlesArgumentsAndEnvironment(t *testing.T) {
	if os.Getenv("WINTRAY_CONSOLE_LIVE_TEST") != "1" {
		t.Skip("set WINTRAY_CONSOLE_LIVE_TEST for owned console-window checks")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINTRAY_RUNNER_CONSOLE_VALUE", "console value 中文")
	want := []string{"", "two words", `C:\My Path\`, `say "hello"`, "中文", ""}
	for _, test := range []struct {
		name             string
		mode             launchMode
		console, visible bool
	}{{"visible", launchVisible, true, true}, {"hidden-console", launchHiddenConsole, true, false}, {"no-window", launchNoWindow, false, false}} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "console.json")
			command, err := startProcess(self, runnerHelperArgs("console", output, want...), test.mode)
			if err != nil {
				t.Fatal(err)
			}
			waitRunnerCommand(t, command)
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Stdin, Stdout, Stderr, Window, Visible bool
				Cwd, Environment                       string
				Args                                   []string
			}
			if err = json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Window != test.console || got.Stdin != test.console || got.Stdout != test.console || got.Stderr != test.console || got.Visible != test.visible {
				t.Fatalf("console lifecycle: %+v", got)
			}
			if got.Cwd != filepath.Dir(self) || got.Environment != "console value 中文" || !reflect.DeepEqual(got.Args, want) {
				t.Fatalf("console launch changed arguments, cwd or environment: %+v", got)
			}
		})
	}
}

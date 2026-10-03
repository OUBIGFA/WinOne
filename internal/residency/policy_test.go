package residency

import "testing"

func TestLoginDisplayPolicy(t *testing.T) {
	for _, test := range []struct {
		name                                string
		autorun, minimized, exitAfter, want bool
	}{
		{"manual host must not open a second window", false, false, false, false},
		{"login opens settings when requested", true, false, false, true},
		{"tray login stays hidden", true, true, false, false},
		{"exit-after-login stays hidden", true, false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if ShowWindowAtLogin(test.autorun, test.minimized, test.exitAfter) != test.want {
				t.Fatal("incorrect login display")
			}
		})
	}
}

func TestExitRequiresExplicitRequestAndNoBlockers(t *testing.T) {
	for _, test := range []struct {
		state State
		want  bool
	}{
		{State{}, false}, {State{Requested: true}, true},
		{State{Requested: true, Reasons: []string{"still running"}}, false},
	} {
		if test.state.CanExit() != test.want {
			t.Fatalf("unexpected decision: %+v", test.state)
		}
	}
}

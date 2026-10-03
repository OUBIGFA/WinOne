// Package residency describes why WinOne must remain running after login tasks finish.
package residency

type State struct {
	Requested bool     `json:"requested"`
	Reasons   []string `json:"reasons"`
}

func (s State) CanExit() bool { return s.Requested && len(s.Reasons) == 0 }

// Login tasks configured to exit after finishing must not open a window that keeps them alive.
func ShowWindowAtLogin(autorun, minimized, exitAfter bool) bool {
	return autorun && !minimized && !exitAfter
}

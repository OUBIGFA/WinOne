//go:build windows

package adminstartup

import (
	"errors"
	"testing"
	"winone/internal/logintask"
)

type launcherFixture struct {
	ready bool
	err   error
	runs  int
	mode  logintask.LaunchMode
}

func (l *launcherFixture) Configured() (bool, error) { return l.ready, l.err }
func (l *launcherFixture) Run(mode logintask.LaunchMode) (string, error) {
	l.runs++
	l.mode = mode
	return "accepted", nil
}
func TestRememberedStartupRequiresExistingAuthorization(t *testing.T) {
	for _, launcher := range []*launcherFixture{{}, {err: errors.New("task changed")}} {
		if err := StartAuthorized(logintask.LoginLaunch, launcher); err == nil || launcher.runs != 0 {
			t.Fatal(launcher, err)
		}
	}
	launcher := &launcherFixture{ready: true}
	if err := StartAuthorized(logintask.LoginLaunch, launcher); err != nil || launcher.runs != 1 || launcher.mode != logintask.LoginLaunch {
		t.Fatal(launcher, err)
	}
}

//go:build windows

package adminstartup

import (
	"context"
	"testing"
	"time"
	"winone/internal/logintask"
)

type revocationFixture struct {
	ready, finish bool
	runs, removes int
	mode          logintask.LaunchMode
}

func (r *revocationFixture) Configured() (bool, error) { return r.ready, nil }
func (r *revocationFixture) Run(mode logintask.LaunchMode) (string, error) {
	r.runs++
	r.mode = mode
	if r.finish {
		r.ready = false
	}
	return "accepted", nil
}
func (r *revocationFixture) Remove() error { r.removes++; r.ready = false; return nil }
func TestRememberedAuthorizationCanRevokeItselfWithoutNewConsent(t *testing.T) {
	task := &revocationFixture{ready: true, finish: true}
	if err := RevokeAuthorized(context.Background(), task, false); err != nil {
		t.Fatal(err)
	}
	if task.ready || task.runs != 1 || task.removes != 0 || task.mode != logintask.RevokeAuthorization {
		t.Fatal(task)
	}
	if err := RevokeAuthorized(context.Background(), task, false); err != nil || task.runs != 1 {
		t.Fatal(task, err)
	}
}
func TestRevocationDoesNotTreatTaskAcceptanceAsRemoval(t *testing.T) {
	task := &revocationFixture{ready: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := RevokeAuthorized(ctx, task, false); err == nil || !task.ready {
		t.Fatal(task, err)
	}
}

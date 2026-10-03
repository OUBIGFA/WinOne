//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAdministratorRestartWaitsForHelperAndCommitAcknowledgement(t *testing.T) {
	token := strings.Repeat("a", 32)
	launched, committed, closed := false, false, false
	statuses := 0
	err := performAdministratorRestart(context.Background(), administratorRestartPorts{
		guard: func() error { return nil }, launch: func() error { launched = true; return nil }, close: func(string) error {
			if !committed || statuses < 2 {
				t.Fatal("closed before acknowledgement")
			}
			closed = true
			return nil
		},
		call: func(method string, _ any) (administratorHandoffSnapshot, error) {
			state := administratorHandoffSnapshot{Token: token, State: "prepared"}
			switch method {
			case "restart.prepare":
			case "restart.status":
				statuses++
				if !launched {
					t.Fatal("status poll before launch")
				}
				state.HelperPID = 20
				if committed {
					state.State = "committed"
					state.HelperAcknowledged = true
				}
			case "restart.commit":
				if statuses == 0 {
					t.Fatal("committed before helper confirmation")
				}
				committed = true
				state.State = "committed"
				state.HelperPID = 20
			default:
				t.Fatal(method)
			}
			return state, nil
		},
	})
	if err != nil || !closed {
		t.Fatal(closed, err)
	}
}
func TestAdministratorRestartCancelsPreparationWhenLaunchFails(t *testing.T) {
	failure := errors.New("scheduler unavailable")
	cancelled := false
	token := strings.Repeat("a", 32)
	err := performAdministratorRestart(context.Background(), administratorRestartPorts{guard: func() error { return nil }, launch: func() error { return failure }, close: func(string) error { t.Fatal("failed restart closed UI"); return nil }, call: func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method == "restart.cancel" {
			cancelled = true
		} else if method != "restart.prepare" {
			t.Fatal(method)
		}
		return administratorHandoffSnapshot{Token: token, State: "prepared"}, nil
	}})
	if !errors.Is(err, failure) || !cancelled {
		t.Fatal(cancelled, err)
	}
}
func TestAdministratorRestartDoesNotCloseAfterUnconfirmedHelper(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	cancelled := false
	token := strings.Repeat("a", 32)
	err := performAdministratorRestart(ctx, administratorRestartPorts{guard: func() error { return nil }, launch: func() error { return nil }, close: func(string) error { t.Fatal("unconfirmed restart closed UI"); return nil }, call: func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method == "restart.cancel" {
			cancelled = true
		}
		if method == "restart.commit" {
			t.Fatal("unconfirmed helper committed")
		}
		return administratorHandoffSnapshot{Token: token, State: "prepared"}, nil
	}})
	if !errors.Is(err, context.DeadlineExceeded) || !cancelled {
		t.Fatal(cancelled, err)
	}
}
func TestAdministratorRestartPreservesChangesMadeBeforeCommit(t *testing.T) {
	checks := 0
	cancelled := false
	failure := errors.New("unsaved changes")
	token := strings.Repeat("a", 32)
	err := performAdministratorRestart(context.Background(), administratorRestartPorts{guard: func() error {
		checks++
		if checks > 1 {
			return failure
		}
		return nil
	}, launch: func() error { return nil }, close: func(string) error { t.Fatal("unsaved UI closed"); return nil }, call: func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method == "restart.cancel" {
			cancelled = true
		}
		if method == "restart.commit" {
			t.Fatal("unsaved changes committed")
		}
		return administratorHandoffSnapshot{Token: token, State: "prepared", HelperPID: 20}, nil
	}})
	if !errors.Is(err, failure) || !cancelled {
		t.Fatal(cancelled, err)
	}
}

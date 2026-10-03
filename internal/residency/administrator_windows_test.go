//go:build windows

package residency

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestAdministratorStartPresenceIsSeparateAndReleased(t *testing.T) {
	pipe := fmt.Sprintf("WinOne-admin-presence-%d-%d", os.Getpid(), time.Now().UnixNano())
	first, err := MarkAdministratorStart(pipe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first)
	second, err := MarkAdministratorStart(pipe)
	if err != nil {
		first()
		t.Fatal(err)
	}
	t.Cleanup(second)
	if present, err := AdministratorStarting(pipe); err != nil || !present {
		t.Fatal(present, err)
	}
	if present, err := WindowOpen(pipe); err != nil || present {
		t.Fatal("helper was mistaken for a UI window", err)
	}
	first()
	if present, err := AdministratorStarting(pipe); err != nil || !present {
		t.Fatal(present, err)
	}
	second()
	if present, err := AdministratorStarting(pipe); err != nil || present {
		t.Fatal(present, err)
	}
}

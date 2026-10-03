//go:build windows

package residency

import (
	"fmt"
	"testing"
	"time"
)

func TestWindowPresenceTracksAllHandles(t *testing.T) {
	pipe := fmt.Sprintf("residency-test-%d", time.Now().UnixNano())
	first, err := MarkWindow(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := MarkWindow(pipe)
	if err != nil {
		t.Fatal(err)
	}
	second()
	if open, err := WindowOpen(pipe); err != nil || !open {
		t.Fatalf("first window lost: %t %v", open, err)
	}
}

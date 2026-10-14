//go:build windows

package transport

import (
	"context"
	"testing"
	"time"
)

func TestChildStdoutDeadlineAndCloseReleaseAStalledAcknowledgement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	child, parent, err := ChildStdout(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	defer parent.Close()
	if err := parent.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var ack [1]byte
	_, err = parent.Read(ack[:])
	if timeout, ok := err.(interface{ Timeout() bool }); !ok || !timeout.Timeout() {
		t.Fatal("stalled acknowledgement did not time out", err)
	}
	if err := parent.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := parent.Read(ack[:]); done <- err }()
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed pipe accepted a reply")
		}
	case <-ctx.Done():
		t.Fatal("closing the parent did not release its reader")
	}
}

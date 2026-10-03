package capturehandoff

import (
	"fmt"
	"strings"
	"testing"
)

func TestQueueRefusesOverflowWithoutDroppingExistingWork(t *testing.T) {
	var queue Queue
	for i := 0; i < 8; i++ {
		if err := queue.Push(fmt.Sprintf("%032x", i)); err != nil {
			t.Fatal(err)
		}
	}
	first := queue.Peek()
	if err := queue.Push(strings.Repeat("f", 32)); err == nil {
		t.Fatal("unbounded request queue")
	}
	if queue.Peek().ID != first.ID {
		t.Fatal("overflow dropped the original request")
	}
}

func TestQueueKeepsUnacknowledgedRequests(t *testing.T) {
	var queue Queue
	a, b := strings.Repeat("a", 32), strings.Repeat("b", 32)
	if err := queue.Push(a); err != nil {
		t.Fatal(err)
	}
	first := queue.Peek()
	if err := queue.Push(a); err != nil {
		t.Fatal(err)
	}
	if err := queue.Push(b); err != nil {
		t.Fatal(err)
	}
	if queue.Peek().ID != first.ID {
		t.Fatal("new launch replaced the pending request")
	}
	if queue.Acknowledge("stale") {
		t.Fatal("stale acknowledgement removed a request")
	}
	if !queue.Acknowledge(first.ID) || queue.Peek().DraftID != b {
		t.Fatal("queue lost order or duplicated a draft")
	}
	if queue.Acknowledge(first.ID) {
		t.Fatal("old acknowledgement removed the next request")
	}
}

func TestDraftArgumentsAreStrict(t *testing.T) {
	id := strings.Repeat("c", 32)
	for _, args := range [][]string{{"--pipe", "test", "--capture-draft", id}, {"WinOne.exe", "--capture-draft=" + id}} {
		got, err := DraftFromArgs(args)
		if err != nil || got != id {
			t.Fatal(got, err)
		}
	}
	for _, args := range [][]string{{"--capture-draft"}, {"--capture-draft", "../../source.png"}, {"--capture-draft=" + id, "--capture-draft=" + id}} {
		if _, err := DraftFromArgs(args); err == nil {
			t.Fatal("invalid launch arguments accepted", args)
		}
	}
	var queue Queue
	if err := queue.Push("bad"); err == nil || queue.Peek() != nil {
		t.Fatal("invalid draft changed queue")
	}
}

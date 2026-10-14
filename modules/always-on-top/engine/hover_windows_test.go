//go:build windows

package engine

import "testing"

func TestHoverTargetDoesNotFallBackToForeground(t *testing.T) {
	calls := []string{}
	target, err := hoverTarget(func() (uintptr, error) { calls = append(calls, "pointer"); return 42, nil }, func(h uintptr) uintptr {
		calls = append(calls, "root")
		if h != 42 {
			t.Fatal(h)
		}
		return 84
	}, func(h uintptr) (Window, error) {
		calls = append(calls, "inspect")
		if h != 84 {
			t.Fatal(h)
		}
		return Window{Handle: "84"}, nil
	})
	if err != nil || target.Handle != "84" || len(calls) != 3 {
		t.Fatalf("target=%+v calls=%v error=%v", target, calls, err)
	}
	_, err = hoverTarget(func() (uintptr, error) { return 0, nil }, func(h uintptr) uintptr { t.Fatal("invalid pointer was resolved"); return 0 }, func(h uintptr) (Window, error) { t.Fatal("invalid pointer was inspected"); return Window{}, nil })
	if err == nil {
		t.Fatal("empty pointer target accepted")
	}
}

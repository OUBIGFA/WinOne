//go:build windows

package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCaptureWindowLifecycleReusesActiveWindowAndReportsItsActualExit(t *testing.T) {
	done := make(chan error, 1)
	starts := 0
	windows := newCaptureWindows(func(string) (<-chan error, error) { starts++; return done, nil })
	var group sync.WaitGroup
	ids := make(chan string, 12)
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := windows.Open(strings.Repeat("a", 32))
			if err != nil {
				t.Error(err)
				return
			}
			ids <- value.ID
		}()
	}
	group.Wait()
	close(ids)
	var id string
	for value := range ids {
		if id == "" {
			id = value
		}
		if value != id {
			t.Fatal("duplicate capture windows")
		}
	}
	if starts != 1 {
		t.Fatal(starts)
	}
	if _, err := windows.Open(strings.Repeat("b", 32)); err == nil {
		t.Fatal("another session replaced an active screenshot")
	}
	done <- nil
	deadline := time.Now().Add(time.Second)
	for {
		status, err := windows.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("window exit was not observed")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := windows.Status("unknown"); err == nil {
		t.Fatal("unknown window reported as complete")
	}
}

func TestCaptureWindowStartAndExitFailuresRemainVisible(t *testing.T) {
	failure := errors.New("cannot start")
	windows := newCaptureWindows(func(string) (<-chan error, error) { return nil, failure })
	if _, err := windows.Open(strings.Repeat("a", 32)); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	windows = newCaptureWindows(func(string) (<-chan error, error) { return done, nil })
	status, err := windows.Open(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	done <- errors.New("window crashed")
	deadline := time.Now().Add(time.Second)
	for {
		value, _ := windows.Status(status.ID)
		if value.State == "failed" {
			if value.Error != "window crashed" {
				t.Fatal(value)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed window reported as running")
		}
		time.Sleep(time.Millisecond)
	}
}

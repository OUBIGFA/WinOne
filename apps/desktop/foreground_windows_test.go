//go:build windows

package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestForegroundPermissionGoesOnlyToVerifiedExistingInstance(t *testing.T) {
	var calls []string
	port := foregroundPermissionPort{
		find: func(id string) (uint32, error) {
			if id != "instance" {
				t.Fatal(id)
			}
			return 42, nil
		},
		verify: func(pid uint32, path string) (func(), error) {
			if pid != 42 || path != "WinOne.exe" {
				t.Fatal(pid, path)
			}
			calls = append(calls, "verify")
			return func() { calls = append(calls, "release") }, nil
		},
		allow: func(pid uint32) error {
			if pid != 42 {
				t.Fatal("wildcard or incorrect foreground target")
			}
			calls = append(calls, "allow")
			return nil
		},
	}
	if err := transferForegroundPermission("instance", "WinOne.exe", port); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"verify", "allow", "release"}) {
		t.Fatal(calls)
	}
	calls = nil
	port.verify = func(uint32, string) (func(), error) { return nil, errors.New("other installation") }
	if err := transferForegroundPermission("instance", "WinOne.exe", port); err == nil {
		t.Fatal("unverified process accepted")
	}
	if len(calls) != 0 {
		t.Fatal(calls)
	}
	port.find = func(string) (uint32, error) { return 0, nil }
	if err := transferForegroundPermission("instance", "WinOne.exe", port); err != nil {
		t.Fatal("cold launch should not require an old window", err)
	}
}

func TestDeniedForegroundPermissionStillReleasesProcessIdentity(t *testing.T) {
	released := false
	denied := errors.New("foreground permission denied")
	port := foregroundPermissionPort{find: func(string) (uint32, error) { return 42, nil }, verify: func(uint32, string) (func(), error) { return func() { released = true }, nil }, allow: func(uint32) error { return denied }}
	if err := transferForegroundPermission("instance", "WinOne.exe", port); !errors.Is(err, denied) || !released {
		t.Fatal(err, released)
	}
}

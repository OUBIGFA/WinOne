//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrayFailureRecordsReasonAndRequestsLocalizedFeedback(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		t.Run(locale, func(t *testing.T) {
			directory := t.TempDir()
			calls := 0
			failure := errors.New("target window disappeared")
			err := reportTrayFailure(directory, locale, failure, func(title, message string) error {
				calls++
				if title != "WinOne" || strings.Contains(message, failure.Error()) {
					t.Fatal(title, message)
				}
				if locale == "en-US" && !strings.Contains(message, "Diagnostic logs") {
					t.Fatal(message)
				}
				if locale == "zh-CN" && !strings.Contains(message, "诊断日志") {
					t.Fatal(message)
				}
				return nil
			})
			if err != nil || calls != 1 {
				t.Fatal(calls, err)
			}
			data, err := os.ReadFile(filepath.Join(directory, "logs", "host.log"))
			if err != nil || !strings.Contains(string(data), failure.Error()) || !strings.Contains(string(data), "tray-action-failed") {
				t.Fatal(string(data), err)
			}
		})
	}
}

func TestTrayFeedbackDoesNotHideItsOwnFailures(t *testing.T) {
	directory := t.TempDir()
	rejected := errors.New("notification rejected")
	if err := reportTrayFailure(directory, "en-US", errors.New("action failed"), func(string, string) error { return rejected }); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	called := false
	err := reportTrayFailure(filepath.Join(directory, "missing"), "en-US", errors.New("action failed"), func(_ string, message string) error {
		called = true
		if message != "action failed" {
			t.Fatal(message)
		}
		return nil
	})
	if err == nil || !called {
		t.Fatal("missing log directory prevented feedback", err)
	}
	if err := reportTrayFailure(directory, "en-US", nil, func(string, string) error { t.Fatal("success notification"); return nil }); err != nil {
		t.Fatal(err)
	}
}

//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestDesktopBootstrapRejectsUnknownAndIncompleteRequests(t *testing.T) {
	for _, input := range []string{`{}`, `{"pipe":"remote","dataDirectory":"relative"}`, `{"pipe":"\\\\.\\pipe\\WinOne-test","dataDirectory":"C:\\test","extra":true}`} {
		var output bytes.Buffer
		if handled, err := desktopBootstrap([]string{"--desktop-bootstrap"}, filepath.Join(t.TempDir(), "WinOne.Host.exe"), bytes.NewBufferString(input), &output); !handled || err == nil || output.Len() != 0 {
			t.Fatal(handled, err, output.String())
		}
	}
	if handled, err := desktopBootstrap([]string{"--desktop-helper", "--not-a-helper"}, "", nil, nil); !handled || err == nil {
		t.Fatal(handled, err)
	}
}

func TestDesktopBootstrapLeavesIsolatedProfileAuthorizationUntouched(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	directory := t.TempDir()
	input, _ := json.Marshal(map[string]any{"pipe": `\\.\pipe\WinOne-test-bootstrap`, "dataDirectory": directory, "standardSession": false})
	var output bytes.Buffer
	handled, err := desktopBootstrap([]string{"--desktop-bootstrap"}, filepath.Join(t.TempDir(), "WinOne.Host.exe"), bytes.NewReader(input), &output)
	if !handled || err != nil {
		t.Fatal(handled, err)
	}
	var decision struct {
		Version         int
		Redirected      bool
		Warning         string
		StandardSession bool
	}
	if err := json.Unmarshal(output.Bytes(), &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Version != 1 || decision.Redirected || decision.Warning != "" || decision.StandardSession {
		t.Fatal(decision)
	}
}

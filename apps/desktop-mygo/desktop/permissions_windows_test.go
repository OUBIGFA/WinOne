//go:build windows

package desktop

import (
	"bytes"
	"encoding/json"
	"errors"

	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"winone/desktopmygo/host"
	"winone/internal/protocol"
)

// frames encodes the helper's messages, numbered from 1.
func frames(t *testing.T, messages ...map[string]any) *bytes.Buffer {
	t.Helper()
	var stream bytes.Buffer
	for i, message := range messages {
		message["version"], message["sequence"] = 1, i+1
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.WriteFrame(&stream, data); err != nil {
			t.Fatal(err)
		}
	}
	return &stream
}

func never() bool { return false }

func code(err error) string {
	var reported *host.Error
	if errors.As(err, &reported) {
		return reported.Code
	}
	return ""
}

func TestCancelledPermissionStartsNoHelper(t *testing.T) {
	command := exec.Command(filepath.Join(t.TempDir(), "missing-permission-helper.exe"))
	_, err := runHelper(command, []byte(`{}`), PermissionAction{Restart: true}, nil, nil, func() bool { return true }, time.Second)
	if code(err) != "cancelled" {
		t.Fatalf("error %v", err)
	}
	if command.Process != nil {
		t.Fatal("a cancelled operation started the helper")
	}
}

func TestPreferenceRelayKeepsTheSavedSettingsAndRejectsUnverifiedResults(t *testing.T) {
	settings := map[string]any{"revision": 8, "startup": map[string]any{"preferAdministrator": true}, "unknown": "kept"}
	var relayed []string
	call := func(method string, params json.RawMessage) (json.RawMessage, error) {
		relayed = append(relayed, method)
		return params, nil
	}
	var replies bytes.Buffer
	result, err := exchange(frames(t,
		map[string]any{"kind": "guard"},
		map[string]any{"kind": "call", "method": "settings.save", "params": settings},
		map[string]any{"kind": "done", "result": map[string]any{"closing": false, "settings": settings}},
	), &replies, PermissionAction{Enabled: true}, call, func() bool { return true }, never)
	if err != nil {
		t.Fatal(err)
	}
	if len(relayed) != 1 || relayed[0] != "settings.save" || !strings.Contains(string(result), `"unknown":"kept"`) {
		t.Fatalf("relayed %v, result %s", relayed, result)
	}
	// The guard's answer and the saved settings went back to the helper.
	first, _ := protocol.ReadFrame(&replies)
	if !strings.Contains(string(first), `"result":true`) {
		t.Fatalf("guard reply %s", first)
	}
	for _, message := range []map[string]any{
		{"kind": "done", "result": map[string]any{"closing": true, "token": strings.Repeat("a", 32)}},
		{"kind": "call", "method": "modules.setEnabled", "params": map[string]any{}},
	} {
		_, err := exchange(frames(t, message), &bytes.Buffer{}, PermissionAction{Restart: true}, func(string, json.RawMessage) (json.RawMessage, error) {
			t.Fatal("an unauthorized request was relayed")
			return nil, nil
		}, func() bool { return true }, never)
		if code(err) != "protocol" {
			t.Fatalf("accepted %v: %v", message, err)
		}
	}
}

// The Host omits preferAdministrator when false: turning the preference
// off is a confirmed result too (permissions.rs rejected it).
func TestPreferenceOffIsConfirmedWithoutTheField(t *testing.T) {
	settings := map[string]any{"revision": 9, "startup": map[string]any{"runAtLogon": false, "startMinimized": true}}
	call := func(string, json.RawMessage) (json.RawMessage, error) { return json.Marshal(settings) }
	_, err := exchange(frames(t,
		map[string]any{"kind": "call", "method": "settings.save", "params": settings},
		map[string]any{"kind": "done", "result": map[string]any{"closing": false, "settings": settings}},
	), &bytes.Buffer{}, PermissionAction{Enabled: false}, call, func() bool { return true }, never)
	if err != nil {
		t.Fatal(err)
	}
	_, err = exchange(frames(t,
		map[string]any{"kind": "call", "method": "settings.save", "params": settings},
		map[string]any{"kind": "done", "result": map[string]any{"closing": false, "settings": settings}},
	), &bytes.Buffer{}, PermissionAction{Enabled: true}, call, func() bool { return true }, never)
	if code(err) != "protocol" {
		t.Fatalf("a preference saved off confirmed turning it on: %v", err)
	}
}

func TestRestartRequiresTheSameCommittedAndAcknowledgedToken(t *testing.T) {
	token := strings.Repeat("a", 32)
	for _, acknowledge := range []bool{false, true} {
		messages := []map[string]any{{"kind": "call", "method": "application.restart.prepare"}, {"kind": "call", "method": "application.restart.commit"}}
		if acknowledge {
			messages = append(messages, map[string]any{"kind": "call", "method": "application.restart.status"})
		}
		messages = append(messages, map[string]any{"kind": "done", "result": map[string]any{"closing": true, "token": token}})
		_, err := exchange(frames(t, messages...), &bytes.Buffer{}, PermissionAction{Restart: true}, func(method string, _ json.RawMessage) (json.RawMessage, error) {
			state := "committed"
			if strings.HasSuffix(method, "prepare") {
				state = "prepared"
			}
			return json.Marshal(map[string]any{"token": token, "state": state, "helperPid": 9, "helperAcknowledged": strings.HasSuffix(method, "status")})
		}, func() bool { return true }, never)
		if (err == nil) != acknowledge {
			t.Fatalf("acknowledged %v: %v", acknowledge, err)
		}
	}
}

func TestFailedPreferenceSaveIsNeverRetried(t *testing.T) {
	calls := 0
	_, err := exchange(frames(t,
		map[string]any{"kind": "call", "method": "settings.save"},
		map[string]any{"kind": "call", "method": "settings.save"},
	), &bytes.Buffer{}, PermissionAction{Enabled: true}, func(string, json.RawMessage) (json.RawMessage, error) {
		calls++
		return nil, &host.Error{Code: "conflict", Message: "settings changed"}
	}, func() bool { return true }, never)
	if err == nil || calls != 1 {
		t.Fatalf("calls %d, error %v", calls, err)
	}
}

func TestHelperErrorIsReportedAsItsHostError(t *testing.T) {
	_, err := exchange(frames(t,
		map[string]any{"kind": "done", "error": map[string]any{"code": "permission", "message": "此配置是独立测试配置"}},
	), &bytes.Buffer{}, PermissionAction{Enabled: true}, nil, func() bool { return true }, never)
	if code(err) != "permission" || !strings.Contains(err.Error(), "独立测试配置") {
		t.Fatalf("error %v", err)
	}
}

// A helper that stops answering is ended at the time limit, even when a
// process it started keeps its output pipe open: here cmd.exe waits for a
// silent PowerShell that inherited the pipe.
func TestStalledHelperIsEndedAtTheLimit(t *testing.T) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(filepath.Join(system, "cmd.exe"), "/c", powershell+" -NoProfile -NonInteractive -Command Start-Sleep -Seconds 8")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	started := time.Now()
	_, err = runHelper(command, []byte(`{"test":true}`), PermissionAction{Restart: true}, nil, func() bool { return true }, never, 300*time.Millisecond)
	if code(err) != "timeout" {
		t.Fatalf("error %v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("ending the helper took %v", elapsed)
	}
}

//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/protocol"
)

func TestDesktopControlRejectsIsolatedAndMalformedInputBeforeConsent(t *testing.T) {
	for _, input := range []string{`{}`, `{"version":1,"operation":"preference","enabled":true,"ownerPID":1,"pipe":"invalid","dataDirectory":"isolated"}`, `{"version":1,"operation":"restart","ownerPID":1,"extra":true}`} {
		var frame, output bytes.Buffer
		if err := protocol.WriteFrame(&frame, []byte(input)); err != nil {
			t.Fatal(err)
		}
		handled, err := desktopControl([]string{"--desktop-control"}, filepath.Join(t.TempDir(), "WinOne.Host.exe"), &frame, &output)
		if !handled || err != nil {
			t.Fatal(handled, err)
		}
		raw, err := protocol.ReadFrame(&output)
		if err != nil {
			t.Fatal(err)
		}
		var message desktopControlMessage
		if err = json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		if message.Kind != "done" || message.Sequence != 1 || message.Error == nil || message.Result != nil {
			t.Fatal(string(raw))
		}
	}
}

func TestDesktopControlRelayRejectsAmbiguousOrMismatchedReplies(t *testing.T) {
	for _, reply := range []string{`{"version":1,"sequence":2,"result":true}`, `{"version":1,"sequence":1,"result":true,"error":{"code":"bad","message":"bad"}}`, `{"version":1,"sequence":1}`} {
		var input, output bytes.Buffer
		_ = protocol.WriteFrame(&input, []byte(reply))
		remote := desktopControlRemote{input: &input, output: &output}
		if _, err := remote.exchange("guard", "", nil); err == nil {
			t.Fatal("accepted", reply)
		}
	}
}

func TestDesktopControlPreferenceUsesPostConsentRevision(t *testing.T) {
	enabled := true
	consent := false
	var saved map[string]any
	result, err := performDesktopControl(context.Background(), desktopControlRequest{Operation: "preference", Enabled: &enabled}, desktopControlPorts{
		guard: func() error { return nil },
		configure: func(value bool) error {
			if !value {
				t.Fatal("wrong consent")
			}
			consent = true
			return nil
		},
		call: func(method string, params any) (json.RawMessage, error) {
			if method == "snapshot" {
				if !consent {
					return json.RawMessage(`{"administrator":false,"settings":{"revision":1,"theme":"light"}}`), nil
				}
				return json.RawMessage(`{"administrator":false,"settings":{"revision":2,"theme":"dark","extra":{"keep":true},"startup":{"runAtLogon":true,"startMinimized":false,"unknown":7}}}`), nil
			}
			if method != "settings.save" {
				t.Fatal(method)
			}
			data, _ := json.Marshal(params)
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			return data, nil
		},
	})
	if err != nil || result.Closing || saved["revision"] != float64(2) || saved["theme"] != "dark" {
		t.Fatal(result, err, saved)
	}
	startup := saved["startup"].(map[string]any)
	if startup["preferAdministrator"] != true || startup["unknown"] != float64(7) || startup["startMinimized"] != false || saved["extra"] == nil {
		t.Fatal(saved)
	}
}

func TestDesktopControlCancelledConsentNeverSaves(t *testing.T) {
	enabled := true
	calls := 0
	_, err := performDesktopControl(context.Background(), desktopControlRequest{Operation: "preference", Enabled: &enabled}, desktopControlPorts{
		guard: func() error { return nil }, configure: func(bool) error { return errors.New("consent cancelled") },
		call: func(method string, _ any) (json.RawMessage, error) {
			calls++
			if method != "snapshot" {
				t.Fatal(method)
			}
			return json.RawMessage(`{"administrator":false,"settings":{"revision":1}}`), nil
		},
	})
	if err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestDesktopControlRestartRequiresAcknowledgedHelper(t *testing.T) {
	for _, failLaunch := range []bool{false, true} {
		token := strings.Repeat("a", 32)
		committed, cancelled := false, false
		result, err := performDesktopControl(context.Background(), desktopControlRequest{Operation: "restart"}, desktopControlPorts{
			guard: func() error { return nil }, configure: func(bool) error { return nil },
			launch: func() error {
				if failLaunch {
					return errors.New("launch failed")
				}
				return nil
			},
			call: func(method string, _ any) (json.RawMessage, error) {
				if method == "snapshot" {
					return json.RawMessage(`{"administrator":false,"settings":{"revision":1}}`), nil
				}
				state := "prepared"
				helper := uint32(9)
				ack := false
				switch method {
				case "application.restart.prepare":
					helper = 0
				case "application.restart.commit":
					committed = true
					state = "committed"
					ack = true
				case "application.restart.cancel":
					cancelled = true
					state = "cancelled"
				case "application.restart.status":
				default:
					t.Fatal(method)
				}
				data, _ := json.Marshal(map[string]any{"token": token, "state": state, "helperPid": helper, "helperAcknowledged": ack})
				return data, nil
			},
		})
		if failLaunch {
			if err == nil || result.Closing || !cancelled || committed {
				t.Fatal(result, err, cancelled, committed)
			}
		} else if err != nil || !result.Closing || result.Token != token || !committed {
			t.Fatal(result, err, committed)
		}
	}
}

func TestDesktopControlGuardRunsBeforeConsent(t *testing.T) {
	result, err := performDesktopControl(context.Background(), desktopControlRequest{Operation: "restart"}, desktopControlPorts{guard: func() error { return errors.New("unsaved edits") }})
	if err == nil || result.Closing {
		t.Fatal(result, err)
	}
}

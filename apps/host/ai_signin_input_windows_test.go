//go:build windows

package main

import (
	"context"
	"encoding/json"
	"testing"
	"winone/internal/host"
)

func TestSignInInputFollowsTheCurrentAuthorizationStep(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  aiSignInState
		value   string
		allowed bool
	}{
		{"device code is entered on website", aiSignInState{State: "waiting", Code: "ABCD-EFGH"}, "ABCD-EFGH", false},
		{"callback", aiSignInState{State: "waiting", PasteCallback: true}, "http://localhost/callback?code=fixture", true},
		{"authorization code", aiSignInState{State: "waiting", PasteCode: true}, "fixture-code", true},
		{"key", aiSignInState{State: "waiting", PasteKey: true}, "fixture-key", true},
		{"optional text", aiSignInState{State: "input", Prompt: &aiSignInPrompt{Type: "text", Key: "optional"}}, "", true},
		{"done", aiSignInState{State: "done", PasteCode: true}, "late-code", false},
		{"canceled", aiSignInState{State: "canceled", PasteCode: true}, "late-code", false},
		{"finishing", aiSignInState{State: "finishing", PasteCode: true}, "late-code", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newAIService(context.Background(), t.TempDir(), "unused.exe")
			s.signin = &aiSignInJob{ID: "owned", owner: 42, Status: test.status, input: make(chan string, 1)}
			raw, _ := json.Marshal(map[string]string{"id": "owned", "callback": test.value})
			response := s.handleAccounts(host.Request{ID: "submit", Method: "accounts.signin.submit", PeerPID: 42, Params: raw})
			if (response.Error == nil) != test.allowed {
				t.Fatalf("allowed=%v response=%+v", test.allowed, response)
			}
			if len(s.signin.input) != map[bool]int{true: 1, false: 0}[test.allowed] {
				t.Fatal("unexpected authorization input delivery")
			}
		})
	}
}

func TestSignInOpenedSurvivesAdapterProgressAndHostPolling(t *testing.T) {
	var progress aiSignInProgress
	if err := json.Unmarshal([]byte(`{"signin":{"agent":"grok","state":"waiting","url":"https://example.invalid/device","opened":true}}`), &progress); err != nil {
		t.Fatal(err)
	}
	job := &aiSignInJob{ID: "owned", Status: aiSignInState{State: "starting"}}
	job.observe(progress)
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Status struct {
			Opened bool
			URL    string
		}
	}
	if err := json.Unmarshal(raw, &reply); err != nil || !reply.Status.Opened || reply.Status.URL != "https://example.invalid/device" {
		t.Fatalf("authorization page ownership was lost: %s (%v)", raw, err)
	}
}

func TestSignInInputRemainsPendingUntilValidationOrNextStep(t *testing.T) {
	job := &aiSignInJob{Status: aiSignInState{State: "waiting", PasteCode: true}, InputPending: true}
	job.observe(aiSignInProgress{SignIn: &aiSignInState{State: "waiting", PasteCode: true, URL: "https://example.invalid/updated"}})
	if !job.InputPending {
		t.Fatal("waiting progress released submitted input")
	}
	job.observe(aiSignInProgress{InputError: "invalid authorization code"})
	if job.InputPending || job.InputError == "" {
		t.Fatal("validation failure did not allow correction")
	}
	job.InputPending = true
	job.observe(aiSignInProgress{SignIn: &aiSignInState{State: "input", Prompt: &aiSignInPrompt{Type: "text", Key: "next"}}})
	if job.InputPending {
		t.Fatal("next prompt stayed locked")
	}
	job.InputPending = true
	job.observe(aiSignInProgress{SignIn: &aiSignInState{State: "done"}})
	if job.InputPending || job.Status.State != "finishing" || job.terminal == nil {
		t.Fatal("account finalization changed")
	}
}

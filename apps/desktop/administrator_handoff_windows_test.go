//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"strings"
	"testing"
)

func TestAdministratorHandoffWaitsForCommitAndRetainsItWhenAckReplyIsLost(t *testing.T) {
	h := &administratorHandoff{pid: 20}
	token := strings.Repeat("a", 32)
	if err := h.observe(func(string, any) (administratorHandoffSnapshot, error) {
		return administratorHandoffSnapshot{}, errAdministratorHandoffNotPrepared
	}); err != nil || h.token != "" {
		t.Fatal(err)
	}
	if err := h.observe(func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method != "restart.bootstrap" {
			t.Fatal(method)
		}
		return administratorHandoffSnapshot{Token: token, HelperPID: 20, State: "prepared"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.ready(); err == nil {
		t.Fatal("uncommitted handoff allowed launch")
	}
	err := h.observe(func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method == "restart.status" {
			return administratorHandoffSnapshot{Token: token, HelperPID: 20, State: "committed"}, nil
		}
		if method != "restart.acknowledge" {
			t.Fatal(method)
		}
		return administratorHandoffSnapshot{}, windows.ERROR_BROKEN_PIPE
	})
	if !errors.Is(err, windows.ERROR_BROKEN_PIPE) || h.ready() != nil {
		t.Fatal("confirmed commit was forgotten after host exit", err)
	}
}
func TestAdministratorHandoffRejectsOtherProcessesAndSessions(t *testing.T) {
	token := strings.Repeat("a", 32)
	for _, state := range []administratorHandoffSnapshot{{Token: token, HelperPID: 21, State: "prepared"}, {Token: "bad", HelperPID: 20, State: "prepared"}} {
		h := &administratorHandoff{pid: 20}
		if err := h.observe(func(string, any) (administratorHandoffSnapshot, error) { return state, nil }); err == nil {
			t.Fatal(state)
		}
	}
	h := &administratorHandoff{pid: 20, token: token}
	if err := h.observe(func(string, any) (administratorHandoffSnapshot, error) {
		return administratorHandoffSnapshot{Token: strings.Repeat("b", 32), HelperPID: 20, State: "committed"}, nil
	}); err == nil {
		t.Fatal("different restart accepted")
	}
	if err := h.observe(func(string, any) (administratorHandoffSnapshot, error) {
		return administratorHandoffSnapshot{Token: token, HelperPID: 20, State: "cancelled"}, nil
	}); err == nil {
		t.Fatal("cancelled restart continued")
	}
}
func TestAdministratorHandoffRequiresAnAcknowledgementForTheSameToken(t *testing.T) {
	token := strings.Repeat("a", 32)
	h := &administratorHandoff{pid: 20, token: token}
	err := h.observe(func(method string, _ any) (administratorHandoffSnapshot, error) {
		if method == "restart.status" {
			return administratorHandoffSnapshot{Token: token, HelperPID: 20, State: "committed"}, nil
		}
		return administratorHandoffSnapshot{Token: strings.Repeat("b", 32), HelperPID: 20, State: "committed", HelperAcknowledged: true}, nil
	})
	if err == nil {
		t.Fatal("unrelated acknowledgement accepted")
	}
}

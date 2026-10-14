//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/chat"
	"winone/internal/host"
)

func TestChatControllerRejectsForeignPeersAndPreservesProjectFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	directory := t.TempDir()
	executable, _ := os.Executable()
	c := registerChat(host.New(nil), ctx, directory, executable, newAIService(ctx, directory, executable), func() string { return "zh-CN" })
	c.desktop = executable
	request := func(method string, params any, pid uint32) host.Response {
		raw, _ := json.Marshal(params)
		return c.handle(host.Request{ID: "test", Method: method, Params: raw, PeerPID: pid})
	}
	if response := request("catalog", nil, 0); response.Error == nil {
		t.Fatal("unverified peer accepted")
	}
	project := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "keep.txt")
	os.WriteFile(file, []byte("user file"), 0600)
	response := request("project.add", map[string]string{"path": project}, uint32(os.Getpid()))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var added chat.Project
	if err := json.Unmarshal(response.Result, &added); err != nil {
		t.Fatal(err)
	}
	c.guard = func() (func(), bool) { return nil, false }
	if response := request("project.remove", map[string]string{"id": added.ID}, uint32(os.Getpid())); response.Error == nil {
		t.Fatal("maintenance admitted mutation")
	}
	if len(c.store.Snapshot().Projects) != 1 {
		t.Fatal("rejected mutation changed catalog")
	}
	c.guard = nil
	if response := request("project.remove", map[string]string{"id": added.ID}, uint32(os.Getpid())); response.Error != nil {
		t.Fatal(response.Error)
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "user file" {
		t.Fatal("removing project altered user files", err)
	}
	if c.active() {
		t.Fatal("metadata access started Pi")
	}
}

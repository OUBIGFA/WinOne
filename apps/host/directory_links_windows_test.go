//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"winone/internal/directorylinks"
	"winone/internal/host"
)

func TestDirectoryLinksPreviewBelongsToClientAndIsSingleUse(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("kept"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := directorylinks.New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	handle := directoryLinksHandler(m, func(uint32) error { return nil })
	call := func(pid uint32, method string, value any) host.Response {
		raw, _ := json.Marshal(value)
		return handle(host.Request{Version: 1, ID: method, PeerPID: pid, Method: method, Params: raw})
	}
	preview := call(123, "preview", directorylinks.Request{Source: source, Directory: root, LinkName: "linked.txt", Kind: "hardlink"})
	if preview.Error != nil {
		t.Fatal(preview.Error)
	}
	var plan directorylinks.Plan
	if err = json.Unmarshal(preview.Result, &plan); err != nil {
		t.Fatal(err)
	}
	if r := call(124, "create", map[string]string{"token": plan.ID}); r.Error == nil {
		t.Fatal("different window used preview")
	}
	if _, err := os.Lstat(plan.LinkPath); !os.IsNotExist(err) {
		t.Fatal("rejected client created link")
	}
	if r := call(123, "create", map[string]string{"token": plan.ID}); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := call(123, "create", map[string]string{"token": plan.ID}); r.Error == nil {
		t.Fatal("preview reused")
	}
}

func TestDirectoryLinksHelperRejectsArbitraryCommands(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--directory-links-elevated"}, {"--directory-links-elevated", "cmd.exe", "/c", "1"}, {"--directory-links-elevated", `\\.\pipe\another`, "abcdefghijklmnopqrstuvwx12", "1"}} {
		if handled, err := directoryLinksCommand(args, executable); !handled || err == nil {
			t.Fatal("invalid privileged entry accepted", args, handled, err)
		}
	}
	if handled, _ := directoryLinksCommand([]string{"--headless"}, executable); handled {
		t.Fatal("ordinary startup captured")
	}
}

func TestDirectoryLinksOpenQueuesOnlyPathsAndConsumesInOrder(t *testing.T) {
	m, err := directorylinks.New(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	handle := directoryLinksHandler(m, func(uint32) error { return nil })
	call := func(method string, value any) host.Response {
		raw, _ := json.Marshal(value)
		return handle(host.Request{Version: 1, ID: method, PeerPID: 123, Method: method, Params: raw})
	}
	for _, path := range []string{`C:\first.txt`, `D:\second`} {
		if r := call("open", map[string]any{"paths": []string{path}}); r.Error != nil {
			t.Fatal(r.Error)
		}
	}
	if r := call("open", map[string]any{"paths": []string{"relative.txt"}}); r.Error == nil {
		t.Fatal("relative prefill accepted")
	}
	var value struct {
		Paths     []string `json:"paths"`
		Requested bool     `json:"requested"`
	}
	r := call("pending", nil)
	if err = json.Unmarshal(r.Result, &value); err != nil || len(value.Paths) != 2 || value.Paths[0] != `C:\first.txt` || value.Paths[1] != `D:\second` {
		t.Fatal(value, err)
	}
	r = call("pending", nil)
	json.Unmarshal(r.Result, &value)
	if len(value.Paths) != 0 || value.Requested || len(m.List()) != 0 {
		t.Fatal("queue did not consume, or created a link")
	}
	if r := call("open", map[string]any{"paths": []string{}, "options": map[string]any{"kind": "symlink", "relative": true}}); r.Error != nil {
		t.Fatal(r.Error)
	}
	r = call("pending", nil)
	json.Unmarshal(r.Result, &value)
	if !value.Requested || len(value.Paths) != 0 {
		t.Fatal("option-only open lost", string(r.Result))
	}
}

func TestDirectoryLinksProductionRegistrationRejectsNonDesktopMutationClients(t *testing.T) {
	root := t.TempDir()
	service := host.New(nil)
	if err := registerDirectoryLinks(service, root); err != nil {
		t.Fatal(err)
	}
	r := service.Handle(host.Request{Version: 1, ID: "foreign", PeerPID: uint32(os.Getpid()), Method: "directory-links.preview", Params: json.RawMessage(`{}`)})
	if r.Error == nil {
		t.Fatal("a same-user test executable acquired desktop mutation authority")
	}
	if _, err := os.Stat(filepath.Join(root, "tools")); !os.IsNotExist(err) {
		t.Fatal("rejected client created tool data")
	}
	r = service.Handle(host.Request{Version: 1, ID: "activation", PeerPID: uint32(os.Getpid()), Method: "directory-links.open", Params: json.RawMessage(`{"paths":[]}`)})
	if r.Error != nil {
		t.Fatal("activation-only queue was unnecessarily privileged", r.Error)
	}
}

func TestDirectoryLinksBrowseListsBothObjectTypesWithoutRegisteringOrCreating(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "源文件夹")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "说明.txt")
	if err := os.WriteFile(file, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := directorylinks.New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	handle := directoryLinksHandler(m, func(uint32) error { return nil })
	params, _ := json.Marshal(map[string]any{"directory": root})
	r := handle(host.Request{Version: 1, ID: "browse", PeerPID: 123, Method: "browse", Params: params})
	if r.Error != nil {
		t.Fatalf("folder browsing must be a read-only supported operation: %v", r.Error)
	}
	var result struct {
		Directory string                                    `json:"directory"`
		Entries   []struct{ Name, Path, ObjectType string } `json:"entries"`
	}
	if err := json.Unmarshal(r.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Directory != root || len(result.Entries) != 2 || result.Entries[0].ObjectType != "directory" || result.Entries[1].Path != file {
		t.Fatalf("unexpected folder listing: %+v", result)
	}
	if len(m.List()) != 0 {
		t.Fatal("browsing registered link history")
	}
	if _, err := os.Stat(filepath.Join(root, "history")); !os.IsNotExist(err) {
		t.Fatal("browsing created tool state")
	}
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"winone/internal/directorylinks"
	"winone/internal/host"
)

func TestDirectoryLinksHistoryImportRequiresPreviewOwnerAndConfirmation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := directorylinks.New(filepath.Join(root, "first"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := first.Preview(directorylinks.Request{Source: source, Directory: root, LinkName: "linked.txt", Kind: "hardlink"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := first.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := first.ExportHistoryFile([]string{row.ID}, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := directorylinks.New(filepath.Join(root, "second"))
	if err != nil {
		t.Fatal(err)
	}
	handle := directoryLinksHandler(second, func(uint32) error { return nil })
	call := func(pid uint32, method string, args any) host.Response {
		raw, _ := json.Marshal(args)
		return handle(host.Request{Version: 1, ID: method, PeerPID: pid, Method: method, Params: raw})
	}
	response := call(11, "history.preview", map[string]string{"path": archive})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var preview directorylinks.HistoryImportPreview
	if err = json.Unmarshal(response.Result, &preview); err != nil || preview.ID == "" {
		t.Fatal(preview, err)
	}
	args := map[string]any{"token": preview.ID, "confirmed": true}
	if response = call(12, "history.import", args); response.Error == nil {
		t.Fatal("another window consumed import")
	}
	if response = call(11, "history.import", map[string]any{"token": preview.ID}); response.Error == nil {
		t.Fatal("unconfirmed import accepted")
	}
	if len(second.List()) != 0 {
		t.Fatal("preview/rejected import wrote history")
	}
	if response = call(11, "history.import", args); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response = call(11, "history.import", args); response.Error == nil {
		t.Fatal("token reused")
	}
	if len(second.List()) != 1 {
		t.Fatal("missing record")
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "preserved" {
		t.Fatal("data changed", err)
	}
	response = call(11, "history.export", map[string]any{"ids": []string{second.List()[0].ID}, "directory": root})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
}

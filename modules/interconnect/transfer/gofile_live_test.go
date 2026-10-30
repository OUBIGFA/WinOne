package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in only. It uploads one generated public probe, never user files,
// reuses a task-local guest account, and revokes the remote folder afterwards.
func TestGofileLiveUploadAndRevoke(t *testing.T) {
	if os.Getenv("WINONE_GOFILE_LIVE") != "1" {
		t.Skip("public GoFile probe requires explicit opt-in")
	}
	root := os.Getenv("WINONE_TRANSFER_TEST_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("choose an isolated absolute test root")
	}
	directory := filepath.Join(root, "gofile-live")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := revokeGofileProbe(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "winone-public-probe.txt")
	if err := os.WriteFile(path, []byte("WinOne public GoFile integration probe. No user data.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := specification{Mode: "store_send", StoreOrigin: gofileOrigin, Paths: []string{path}, DataDir: directory, AccountDir: filepath.Join(directory, "account"), ExpirationHours: 1, HTTPProxy: os.Getenv("WINONE_GOFILE_PROXY")}
	transferTask := &task{spec: spec}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, uploadErr := transferTask.gofile(ctx)
	if err := revokeGofileProbe(directory); err != nil {
		t.Errorf("probe revocation failed; receipt retained: %v", err)
	}
	if uploadErr != nil {
		t.Fatal(uploadErr)
	}
	t.Log("GoFile guest account, streaming upload, download-page result and revocation verified")
}

// Clean a previous interrupted probe without creating or uploading anything.
func TestGofileLiveRevokePreviousProbe(t *testing.T) {
	if os.Getenv("WINONE_GOFILE_LIVE") != "1" {
		t.Skip("explicit opt-in required")
	}
	root := os.Getenv("WINONE_TRANSFER_TEST_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("choose an isolated absolute test root")
	}
	if err := revokeGofileProbe(filepath.Join(root, "gofile-live")); err != nil {
		t.Fatal(err)
	}
}
func revokeGofileProbe(directory string) error {
	path := filepath.Join(directory, "gofile-receipt.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var receipt struct {
		ID      string `json:"id"`
		Token   string `json:"uploadToken"`
		Revoked bool   `json:"revoked"`
	}
	if err = json.Unmarshal(data, &receipt); err != nil {
		return err
	}
	if receipt.Revoked {
		return nil
	}
	task := &task{spec: specification{Mode: "store_revoke", StoreOrigin: gofileOrigin, StoreID: receipt.ID, UploadToken: receipt.Token, HTTPProxy: os.Getenv("WINONE_GOFILE_PROXY")}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err = task.gofile(ctx); err != nil {
		return err
	}
	receipt.Revoked = true
	data, err = json.Marshal(receipt)
	if err != nil {
		return err
	}
	return writeGofilePrivate(path, data)
}

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Public probe is explicit and contains no user data. A durable local receipt
// allows cleanup after an interrupted run; successful runs revoke the share.
func TestCrocLiveUploadAndRevoke(t *testing.T) {
	if os.Getenv("WINONE_CROC_LIVE") != "1" {
		t.Skip("explicit public croc probe opt-in required")
	}
	root := os.Getenv("WINONE_TRANSFER_TEST_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("isolated absolute root required")
	}
	dir := filepath.Join(root, "croc-live")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(dir, "receipt.json")
	revoke := func() {
		data, err := os.ReadFile(receiptPath)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Code        string
			UploadToken string
			Revoked     bool
		}
		if err = json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Revoked {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		task := &task{spec: specification{Mode: "store_revoke", StoreOrigin: "https://getcroc.com", Code: receipt.Code, UploadToken: receipt.UploadToken}}
		if _, err = task.stored(ctx); err != nil {
			t.Fatalf("probe revoke failed; receipt retained: %v", err)
		}
		receipt.Revoked = true
		data, _ = json.Marshal(receipt)
		if err = os.WriteFile(receiptPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	revoke()
	path := filepath.Join(dir, "winone-public-probe.txt")
	if err := os.WriteFile(path, []byte("WinOne cloud transfer integration probe. No user data.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := defaults()
	spec.Mode = "store_send"
	spec.Paths = []string{path}
	work, err := os.MkdirTemp(dir, "upload-")
	if err != nil {
		t.Fatal(err)
	}
	spec.DataDir = work
	spec.ExpirationHours = 1
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := (&task{spec: spec}).stored(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(receiptPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	revoke()
	t.Log("croc public encrypted upload and revocation verified")
}

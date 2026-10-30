package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testGofileRoot = "11111111-1111-4111-8111-111111111111"
const testGofileFolder = "22222222-2222-4222-8222-222222222222"
const testGofileToken = "isolated-guest-secret"

func TestGofileGuestUploadReuseAndRevoke(t *testing.T) {
	dir, source, _ := transferPaths(t)
	content := bytes.Repeat([]byte("云端文件"), 1024)
	var accounts, uploads, deletions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		reply := func(data any) { json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data}) }
		if r.URL.Path == "/accounts" {
			accounts.Add(1)
			reply(gofileAccount{Token: testGofileToken, Root: testGofileRoot})
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testGofileToken {
			t.Error("missing private account token")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /contents/createFolder":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["parentFolderId"] != testGofileRoot || body["public"] != true {
				t.Errorf("folder: %+v", body)
			}
			reply(map[string]any{"id": testGofileFolder, "code": "Ab12Cd34"})
		case "POST /uploadfile":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("folderId") != testGofileFolder {
				t.Error("wrong upload folder")
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			got, _ := io.ReadAll(file)
			if !bytes.Equal(got, content) || !strings.HasSuffix(header.Filename, ".txt") {
				t.Error("file bytes or name changed")
			}
			uploads.Add(1)
			reply(map[string]any{"parentFolder": testGofileFolder, "downloadPage": "https://gofile.io/d/Ab12Cd34"})
		case "DELETE /contents":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["contentsId"] != testGofileFolder {
				t.Error("wrong revocation target")
			}
			deletions.Add(1)
			reply(nil)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := &gofileClient{http: server.Client(), api: server.URL, upload: server.URL + "/uploadfile"}
	paths := []string{filepath.Join(source, "资料.txt"), filepath.Join(source, "second.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 2 {
		taskDir := filepath.Join(dir, fmt.Sprint(index))
		os.MkdirAll(taskDir, 0700)
		task := &task{spec: specification{Mode: "store_send", StoreOrigin: gofileOrigin, Paths: paths, DataDir: taskDir, AccountDir: filepath.Join(dir, "account"), ExpirationHours: 1}}
		result, err := task.runGofile(context.Background(), client)
		if err != nil {
			t.Fatal(err)
		}
		receipt := result.(map[string]any)
		if receipt["link"] != "https://gofile.io/d/Ab12Cd34" || receipt["qr"] == "" || receipt["uploadToken"] != testGofileToken || receipt["downloads"] != nil || receipt["expiresAt"] != nil || receipt["code"] != nil {
			t.Fatalf("incorrect result: %+v", receipt)
		}
		if task.state.StoreProgress.TotalBytes != int64(len(content)*2) {
			t.Fatal("incorrect upload progress")
		}
		saved, err := os.ReadFile(filepath.Join(taskDir, "gofile-receipt.json"))
		if err != nil || !bytes.Contains(saved, []byte(testGofileFolder)) {
			t.Fatal("revocation receipt missing", err)
		}
	}
	encrypted, err := os.ReadFile(filepath.Join(dir, "account", "account.bin"))
	if err != nil || bytes.Contains(encrypted, []byte(testGofileToken)) {
		t.Fatal("account not encrypted", err)
	}
	revoke := &task{spec: specification{Mode: "store_revoke", StoreID: testGofileFolder, UploadToken: testGofileToken}}
	if _, err := revoke.runGofile(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if accounts.Load() != 1 || uploads.Load() != 4 || deletions.Load() != 1 {
		t.Fatal("unexpected API sequence", accounts.Load(), uploads.Load(), deletions.Load())
	}
}

func TestGofileExpiredGuestIsRenewedOnlyBeforeUpload(t *testing.T) {
	dir, source, _ := transferPaths(t)
	var accounts, uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		reply := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data}) }
		if r.URL.Path == "/accounts" {
			token := "expired-token"
			if accounts.Add(1) > 1 {
				token = "replacement-token"
			}
			reply(gofileAccount{Token: token, Root: testGofileRoot})
			return
		}
		if r.Header.Get("Authorization") == "Bearer expired-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"status":"error-wrongToken"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer replacement-token" {
			t.Error("unexpected account")
		}
		switch r.URL.Path {
		case "/contents/createFolder":
			reply(map[string]any{"id": testGofileFolder, "code": "Ab12Cd34"})
		case "/uploadfile":
			_, _ = io.Copy(io.Discard, r.Body)
			uploads.Add(1)
			reply(map[string]any{"parentFolder": testGofileFolder, "downloadPage": "https://gofile.io/d/Ab12Cd34"})
		default:
			t.Error("unexpected request", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &gofileClient{http: server.Client(), api: server.URL, upload: server.URL + "/uploadfile"}
	accountDir := filepath.Join(dir, "account")
	if _, err := client.account(context.Background(), accountDir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "probe.txt")
	if err := os.WriteFile(file, []byte("public test data"), 0600); err != nil {
		t.Fatal(err)
	}
	uploadTask := &task{spec: specification{Mode: "store_send", Paths: []string{file}, DataDir: dir, AccountDir: accountDir}}
	if _, err := uploadTask.runGofile(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if accounts.Load() != 2 || uploads.Load() != 1 {
		t.Fatal("renewal duplicated uploads", accounts.Load(), uploads.Load())
	}
	// A new guest cannot own an older share. Do not fake revocation by retrying
	// the old folder with replacement credentials or reporting success on 401.
	revoke := &task{spec: specification{Mode: "store_revoke", StoreID: testGofileFolder, UploadToken: "expired-token"}}
	if _, err := revoke.runGofile(context.Background(), client); err == nil || !strings.Contains(err.Error(), "error-wrongToken") {
		t.Fatal("old revocation was not reported accurately", err)
	}
	if accounts.Load() != 2 {
		t.Fatal("revocation unexpectedly renewed the account")
	}
}

func TestGofileConcurrentGuestCreationUsesOneAccount(t *testing.T) {
	dir, _, _ := transferPaths(t)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		time.Sleep(80 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": gofileAccount{Token: testGofileToken, Root: testGofileRoot}})
	}))
	defer server.Close()
	client := &gofileClient{http: server.Client(), api: server.URL}
	var group sync.WaitGroup
	for range 6 {
		group.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := client.account(ctx, filepath.Join(dir, "account")); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if count.Load() != 1 {
		t.Fatal("minted duplicate guest accounts", count.Load())
	}
}

func TestGofileFailedUploadPreservesReceiptAndDoesNotPublishLink(t *testing.T) {
	dir, source, _ := transferPaths(t)
	path := filepath.Join(source, "file.txt")
	os.WriteFile(path, []byte("sample"), 0600)
	for _, stage := range []string{"upload"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch r.URL.Path {
				case "/accounts":
					data = gofileAccount{Token: testGofileToken, Root: testGofileRoot}
				case "/contents/createFolder":
					data = map[string]any{"id": testGofileFolder, "code": "Ab12Cd34"}
				case "/uploadfile":
					io.Copy(io.Discard, r.Body)
					fmt.Fprint(w, `{"status":"error-limits","data":{"token":"DO-NOT-LOG"}}`)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data})
			}))
			defer server.Close()
			client := &gofileClient{http: server.Client(), api: server.URL, upload: server.URL + "/uploadfile"}
			taskDir := filepath.Join(dir, stage)
			os.MkdirAll(taskDir, 0700)
			task := &task{spec: specification{Mode: "store_send", Paths: []string{path}, DataDir: taskDir, AccountDir: filepath.Join(dir, "account"), ExpirationHours: 1}}
			result, err := task.runGofile(context.Background(), client)
			if err == nil || strings.Contains(err.Error(), "DO-NOT-LOG") {
				t.Fatal("API failure was lost or leaked", err)
			}
			receipt, ok := result.(map[string]any)
			if !ok || receipt["uploadToken"] != testGofileToken || receipt["link"] != nil {
				t.Fatalf("partial result: %+v", result)
			}
		})
	}
}

func TestGofileLinksAndRedirectsAreConstrained(t *testing.T) {
	for _, value := range []string{"https://gofile.io/d/ABC123", "https://gofile.io/d/Ab12Cd34"} {
		if !validGofileLink(value) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"https://gofile.io.evil.test/d/ABC123", "https://gofile.io@evil.test/d/ABC123", "http://gofile.io/d/ABC123", "https://gofile.io/d/ABC123?token=secret", "https://gofile.io/d/ABC123#secret", "file:///C:/example", "https://gofile.io/d/../../accounts"} {
		if validGofileLink(value) {
			t.Fatal("unsafe link accepted", value)
		}
	}
	client, err := newGofileClient(specification{})
	if err != nil {
		t.Fatal(err)
	}
	if client.http.CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("redirects expose guest credentials")
	}
	if _, err := newGofileClient(specification{HTTPProxy: "file:///tmp/proxy"}); err == nil {
		t.Fatal("invalid proxy ignored")
	}
}

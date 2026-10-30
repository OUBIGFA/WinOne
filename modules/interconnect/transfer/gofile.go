// GoFile's documented guest API is separate from croc's encrypted protocol.
// Only HTTPS transport encryption applies; the UI must not call it E2EE.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/schollz/croc/v11/src/storeclient"
)

const gofileOrigin = "https://gofile.io"

var gofileID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var gofileCode = regexp.MustCompile(`^(?:[a-zA-Z0-9]{6}|[a-zA-Z0-9]{8})$`)

type gofileAccount struct {
	Token string `json:"token"`
	Root  string `json:"rootFolder"`
}
type gofileAPIError struct {
	Status int
	Code   string
}

func (e *gofileAPIError) Error() string { return fmt.Sprintf("GoFile: HTTP %d (%s)", e.Status, e.Code) }

type gofileClient struct {
	http        *http.Client
	api, upload string
}

func newGofileClient(spec specification) (*gofileClient, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 45 * time.Second
	// Match croc's explicit SOCKS5-first precedence; never ignore invalid input.
	address, scheme := spec.Socks5, "socks5"
	if address == "" {
		address, scheme = spec.HTTPProxy, "http"
	}
	if address != "" {
		if !strings.Contains(address, "://") {
			address = scheme + "://" + address
		}
		proxy, err := url.Parse(address)
		if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") {
			return nil, errors.New("GoFile: invalid proxy address")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &gofileClient{http: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("GoFile: unexpected redirect") }}, api: "https://api.gofile.io", upload: "https://upload.gofile.io/uploadfile"}, nil
}

func (c *gofileClient) request(ctx context.Context, method, path, token string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.api+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return c.do(request, out)
}
func (c *gofileClient) do(request *http.Request, out any) error {
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("GoFile: %w", err)
	}
	defer response.Body.Close()
	var envelope struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("GoFile: response is too large")
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("GoFile: HTTP %d, invalid JSON response", response.StatusCode)
	}
	// Do not echo bodies or arbitrary error text: account responses contain tokens.
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Status != "ok" {
		status := envelope.Status
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`).MatchString(status) {
			status = "invalid-status"
		}
		return &gofileAPIError{Status: response.StatusCode, Code: status}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

// Acquire the process lock before reading/creating a guest account. The same
// account is reused across tasks, even after all prior shares are revoked.
func (c *gofileClient) account(ctx context.Context, directory string) (gofileAccount, error) {
	return c.accountForUpload(ctx, directory, "")
}

// Only a definite rejected guest token may be replaced. Recheck under the
// existing cross-process lock so concurrent uploads reuse one new account.
// Per-share revocation receipts retain their original account token.
func (c *gofileClient) accountForUpload(ctx context.Context, directory, expiredToken string) (gofileAccount, error) {
	var account gofileAccount
	if !filepath.IsAbs(directory) {
		return account, errors.New("GoFile: invalid private account directory")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return account, err
	}
	unlock, err := lockGofileAccount(ctx, filepath.Join(directory, "account.lock"))
	if err != nil {
		return account, err
	}
	defer unlock()
	path := filepath.Join(directory, "account.bin")
	encoded, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err == nil {
		encoded, err = protectGofileSecret(encoded, true)
		if err == nil {
			err = json.Unmarshal(encoded, &account)
		}
	}
	if missing || err == nil && expiredToken != "" && account.Token == expiredToken {
		err = c.request(ctx, "POST", "/accounts", "", map[string]any{}, &account)
		if err == nil && (account.Token == "" || !gofileID.MatchString(account.Root)) {
			err = errors.New("GoFile: incomplete guest account response")
		}
		if err == nil {
			encoded, err = json.Marshal(account)
		}
		if err == nil {
			encoded, err = protectGofileSecret(encoded, false)
		}
		if err == nil {
			err = writeGofilePrivate(path, encoded)
		}
	}
	if err != nil {
		return gofileAccount{}, err
	}
	if account.Token == "" || !gofileID.MatchString(account.Root) {
		return gofileAccount{}, errors.New("GoFile: saved account is invalid; preserve it for recovery")
	}
	return account, nil
}
func writeGofilePrivate(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".gofile-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func validGofileLink(value string) bool {
	link, err := url.Parse(value)
	return err == nil && link.Scheme == "https" && link.Host == "gofile.io" && link.User == nil && link.RawQuery == "" && link.Fragment == "" && strings.HasPrefix(link.Path, "/d/") && gofileCode.MatchString(strings.TrimPrefix(link.Path, "/d/"))
}
func (t *task) gofile(ctx context.Context) (any, error) {
	client, err := newGofileClient(t.spec)
	if err != nil {
		return nil, err
	}
	defer client.http.CloseIdleConnections()
	return t.runGofile(ctx, client)
}
func (t *task) runGofile(ctx context.Context, client *gofileClient) (any, error) {
	if t.spec.Mode == "store_revoke" {
		if !gofileID.MatchString(t.spec.StoreID) || t.spec.UploadToken == "" {
			return nil, errors.New("GoFile: invalid revocation receipt")
		}
		return nil, client.request(ctx, "DELETE", "/contents", t.spec.UploadToken, map[string]any{"contentsId": t.spec.StoreID}, nil)
	}
	if t.spec.Mode != "store_send" {
		return nil, errors.New("GoFile downloads open in its website; API listings and direct links require Premium")
	}
	var total int64
	for _, path := range t.spec.Paths {
		stat, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() {
			return nil, errors.New("GoFile: choose regular files")
		}
		total += stat.Size()
	}
	account, err := client.account(ctx, t.spec.AccountDir)
	if err != nil {
		return nil, err
	}
	var folder struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	createFolder := func() error {
		return client.request(ctx, "POST", "/contents/createFolder", account.Token, map[string]any{"parentFolderId": account.Root, "folderName": "WinOne " + time.Now().Format("2006-01-02 15-04-05"), "public": true}, &folder)
	}
	err = createFolder()
	var rejected *gofileAPIError
	if errors.As(err, &rejected) && rejected.Status == http.StatusUnauthorized && rejected.Code == "error-wrongToken" {
		account, err = client.accountForUpload(ctx, t.spec.AccountDir, account.Token)
		if err == nil {
			err = createFolder()
		}
	}
	if err != nil {
		return nil, err
	}
	if !gofileID.MatchString(folder.ID) {
		return nil, errors.New("GoFile: invalid folder response")
	}
	receipt := map[string]any{"id": folder.ID, "service": "gofile", "uploadToken": account.Token}
	// Save revocation data before sending any file bytes. A failed or cancelled
	// upload still returns the receipt so the user can revoke its partial folder.
	data, err := json.Marshal(receipt)
	if err == nil {
		err = writeGofilePrivate(filepath.Join(t.spec.DataDir, "gofile-receipt.json"), data)
	}
	if err != nil {
		return receipt, fmt.Errorf("GoFile: save revocation receipt: %w", err)
	}
	t.mu.Lock()
	t.state.Result = receipt
	t.mu.Unlock()
	if !gofileCode.MatchString(folder.Code) {
		return receipt, errors.New("GoFile: missing folder share code")
	}
	// Guest accounts cannot configure expiry (403 error-premiumRequired).
	// Retention belongs to GoFile; do not advertise a locally invented deadline.
	var uploaded int64
	for index, path := range t.spec.Paths {
		file, err := os.Open(path)
		if err != nil {
			return receipt, err
		}
		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return receipt, err
		}
		progress := storeclient.Progress{FileIndex: index, FileCount: len(t.spec.Paths), FileName: stat.Name(), FileSize: stat.Size(), TotalSize: total, TotalBytes: uploaded}
		link, err := client.uploadFile(ctx, account.Token, folder.ID, file, stat, func(n int) {
			progress.FileBytes += int64(n)
			progress.TotalBytes += int64(n)
			t.mu.Lock()
			copy := progress
			t.state.StoreProgress = &copy
			t.mu.Unlock()
		})
		err = errors.Join(err, file.Close())
		if err != nil {
			return receipt, err
		}
		if link != "https://gofile.io/d/"+folder.Code {
			return receipt, errors.New("GoFile: upload returned an unexpected download page")
		}
		uploaded += stat.Size()
	}
	// Copy instead of mutating the map shared with snapshot serialization.
	complete := map[string]any{}
	for key, value := range receipt {
		complete[key] = value
	}
	link := "https://gofile.io/d/" + folder.Code
	complete["link"] = link
	complete["qr"] = qrData(link)
	return complete, nil
}

type gofileProgressReader struct {
	io.Reader
	progress func(int)
}

func (r gofileProgressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.progress(n)
	}
	return n, err
}
func (c *gofileClient) uploadFile(ctx context.Context, token, folder string, file *os.File, stat os.FileInfo, progress func(int)) (string, error) {
	// Assemble headers in memory but stream the actual file. A known length also
	// avoids proxies buffering an unbounded chunked upload.
	var header bytes.Buffer
	writer := multipart.NewWriter(&header)
	if err := writer.WriteField("folderId", folder); err != nil {
		return "", err
	}
	if _, err := writer.CreateFormFile("file", stat.Name()); err != nil {
		return "", err
	}
	prefix := append([]byte(nil), header.Bytes()...)
	if err := writer.Close(); err != nil {
		return "", err
	}
	suffix := header.Bytes()[len(prefix):]
	body := io.MultiReader(bytes.NewReader(prefix), gofileProgressReader{Reader: file, progress: progress}, bytes.NewReader(suffix))
	request, err := http.NewRequestWithContext(ctx, "POST", c.upload, body)
	if err != nil {
		return "", err
	}
	request.ContentLength = int64(len(prefix)+len(suffix)) + stat.Size()
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	var result struct {
		Parent string `json:"parentFolder"`
		Link   string `json:"downloadPage"`
	}
	if err = c.do(request, &result); err != nil {
		return "", err
	}
	if result.Parent != folder || !validGofileLink(result.Link) {
		return "", errors.New("GoFile: invalid download response")
	}
	return result.Link, nil
}

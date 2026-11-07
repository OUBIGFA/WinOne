//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/host"
	"winone/internal/processidentity"
)

func TestNativeChatRemoteImagePreviewUsesTheActualHostAndPreservesDraftAuthority(t *testing.T) {
	root := t.TempDir()
	store, err := chat.Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.NewConversation("", "", "Images", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.NewConversation("", "", "Other", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	draft := chat.Draft{Text: "Keep this unsent owner draft"}
	if err = store.SaveDraft(owner.ID, draft); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pid := uint32(os.Getpid())
	peer, err := processidentity.Open(pid, exe)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	c := &chatControl{ctx: ctx, store: store, desktop: exe, owners: map[uint32]*processidentity.Process{pid: peer}}
	t.Cleanup(func() {
		if c.previews != nil {
			c.previews.Close()
		}
	})
	pixels := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	pixels.SetNRGBA(50, 50, color.NRGBA{11, 22, 33, 255})
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/picture.png", http.StatusFound)
		case "/picture.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(encoded.Bytes())
		default:
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "<html>Not an image</html>")
		}
	}))
	defer source.Close()
	call := func(id, url string) host.Response {
		params, _ := json.Marshal(map[string]string{"id": id, "url": url})
		return c.handle(host.Request{ID: "image", PeerPID: pid, Method: "image.preview", Params: params})
	}
	result := call(owner.ID, source.URL+"/redirect")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	var preview chat.FilePreview
	if err = json.Unmarshal(result.Result, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "image" || preview.MIME != "image/png" || !strings.HasPrefix(preview.URL, "http://127.0.0.1:") || strings.Contains(preview.URL, source.URL) {
		t.Fatal("not a Host-issued image capability", preview)
	}
	reply, err := http.Get(preview.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reply.Body)
	reply.Body.Close()
	if err != nil || reply.StatusCode != 200 || !bytes.Equal(data, encoded.Bytes()) {
		t.Fatal("original picture did not reach native preview", err, reply.StatusCode)
	}
	saved, err := store.ReadDraft(owner.ID)
	if err != nil || saved.Text != draft.Text || len(saved.Attachments) != 0 {
		t.Fatal("image display altered the unsent owner input", saved, err)
	}
	if _, err = store.PreviewFile(other.ID, preview.Path); err == nil {
		t.Fatal("another task borrowed the downloaded image file")
	}
	if _, err = store.SavePreviewFile(owner.ID, preview.Path, preview.Revision, "mutated"); err == nil {
		t.Fatal("viewing a remote picture granted file mutation")
	}
	if result = call(owner.ID, source.URL+"/html"); result.Error == nil {
		t.Fatal("HTML exposed as an image")
	}
	for _, url := range []string{"file:///C:/private.png", "https://user:secret@example.invalid/picture.png"} {
		if result = call(owner.ID, url); result.Error == nil {
			t.Fatal("unsupported/credential URL exposed", url)
		}
	}
	before := reads.Load()
	if result = call("missing-task", source.URL+"/picture.png"); result.Error == nil || reads.Load() != before {
		t.Fatal("missing task initiated external read")
	}
	if allowed, readOnly := oneBotMethod("chat.image.preview"); !allowed || !readOnly {
		t.Fatal("shared OneBot native image operation lost its read-only classification")
	}
}

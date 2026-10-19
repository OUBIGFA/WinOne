package chat

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestMediaUploadIsSingleUseAndPreservesPrivateImageBytes(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Images", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetNRGBA(1, 1, color.NRGBA{10, 20, 30, 128})
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	var imported Attachment
	address, err := server.OpenUpload(conversation.ID, func(_ context.Context, data []byte) (any, error) {
		var err error
		imported, err = store.ImportMedia(conversation.ID, "picture.png", data)
		return imported, err
	})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("OPTIONS", address, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal("upload preflight failed", response.StatusCode)
	}
	response, err = http.Post(address, "image/png", bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("upload failed", response.StatusCode)
	}
	stored, err := os.ReadFile(imported.Path)
	if err != nil || !bytes.Equal(stored, encoded.Bytes()) {
		t.Fatal("image bytes were altered", err)
	}
	if err = store.ValidateAttachments(conversation.ID, []string{imported.Path}); err != nil {
		t.Fatal(err)
	}
	response, err = http.Post(address, "image/png", bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("upload capability replay accepted")
	}
	preview, err := server.Open(conversation.ID, imported.Path)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.Post(preview.URL, "image/png", bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 405 {
		t.Fatal("read-only preview became writable")
	}
}

func TestRemoteMediaRejectsNonImagesAndCredentialURLs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Media", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html><script>bad</script><svg></svg></html>")
	}))
	defer fake.Close()
	if _, err = store.ImportRemoteMedia(context.Background(), conversation.ID, fake.URL); err == nil {
		t.Fatal("HTML containing an SVG tag was accepted as an image")
	}
	for _, address := range []string{"file:///C:/private.png", "https://user:secret@example.test/image.png"} {
		if _, err = store.ImportRemoteMedia(context.Background(), conversation.ID, address); err == nil {
			t.Fatal("unsupported media source", address)
		}
	}
	if _, err = store.ImportMedia(conversation.ID, "diagram.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M1 1L8 8"/></svg>`)); err != nil {
		t.Fatal("valid SVG rejected", err)
	}
}

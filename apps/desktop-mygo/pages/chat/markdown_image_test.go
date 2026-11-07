package chat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeMarkdownAndRichPicturesDecodeTheRealHostPreview(t *testing.T) {
	f := newFakeChat(t)
	image := pngBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "owned-image.png")
	if err := os.WriteFile(path, image, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.file.preview" {
			var input struct{ ID, Path string }
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if input.ID != "c1" || input.Path != path {
				return nil, fmt.Errorf("PREVIEW_IMAGE_OWNER_DENIED")
			}
			calls++
			return map[string]any{"kind": "image", "mimeType": "image/png", "url": server.URL, "name": "Owned image"}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	options := p.answerOptions("owned-picture")
	if options.LoadImage == nil {
		t.Fatal("the production Markdown/rich image path has only an alt placeholder")
	}
	source, err := options.LoadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	bitmap, ok := source.(*ui.Bitmap)
	if !ok {
		t.Fatal("the actual Host preview did not use the native bitmap decoder")
	}
	if w, h := bitmap.Size(); w != 2 || h != 2 || calls != 1 {
		t.Fatal("a placeholder or filename was reported as decoded image bytes")
	}
	if _, err := options.LoadImage(filepath.Join(filepath.Dir(path), "foreign.png")); err == nil {
		t.Fatal("a rejected file bypassed the existing Host preview boundary")
	}
}

// A valid private ancillary PNG chunk, with its CRC, increases byte size without
// asking the decoder for a giant pixel allocation. This exercises the original
// 100 MiB byte-action contract, not malformed trailing bytes or a filename.
func markdownImagePNG(t *testing.T, padding int) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, 160, 96))
	for y := range 96 {
		for x := range 160 {
			pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(30 + x), G: uint8(60 + y), B: 100, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	raw := encoded.Bytes()
	if padding == 0 {
		return raw
	}
	at := len(raw) - 12 // Keep the original IEND as the last chunk.
	out := make([]byte, len(raw)+padding+12)
	copy(out, raw[:at])
	binary.BigEndian.PutUint32(out[at:], uint32(padding))
	copy(out[at+4:], "vpAg")
	binary.BigEndian.PutUint32(out[at+8+padding:], crc32.ChecksumIEEE(out[at+4:at+8+padding]))
	copy(out[at+12+padding:], raw[at:])
	return out
}

func TestRemoteMarkdownPicturesUseHostAuthorityForHTTPAndHTTPS(t *testing.T) {
	image := markdownImagePNG(t, 0)
	var reads atomic.Uint32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer server.Close()
	for _, source := range []string{"http://author.example.invalid/picture.png", "https://author.example.invalid/picture.png"} {
		t.Run(strings.Split(source, ":")[0], func(t *testing.T) {
			f := newFakeChat(t)
			calls := 0
			h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
				if method == "chat.image.preview" {
					var input struct{ ID, URL string }
					if err := json.Unmarshal(raw, &input); err != nil {
						return nil, err
					}
					if input.ID != "c1" || input.URL != source {
						return nil, fmt.Errorf("REMOTE_IMAGE_OWNER_DENIED")
					}
					calls++
					return map[string]any{"kind": "image", "mimeType": "image/png", "url": server.URL}, nil
				}
				return f.handle(method, raw)
			})
			open(t, h, p, tt, "c1")
			before := reads.Load()
			bitmap, err := p.answerOptions("remote-" + source).LoadImage(source)
			if err != nil {
				t.Fatal(err)
			}
			decoded, ok := bitmap.(*ui.Bitmap)
			if !ok || calls != 1 || reads.Load() != before+1 {
				t.Fatal("authored URL bypassed the Host or the native byte decoder")
			}
			if w, h := decoded.Size(); w != 160 || h != 96 {
				t.Fatal("remote source produced an alt placeholder instead of its actual pixels")
			}
		})
	}
}

func TestMarkdownImageAcceptsALawfulPNGAboveTheOld64MiBCap(t *testing.T) {
	image := markdownImagePNG(t, 65<<20)
	if len(image) <= 64<<20 || len(image) > 100<<20 {
		t.Fatal("the valid PNG fixture does not exercise the original byte budget")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer server.Close()
	f := newFakeChat(t)
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.image.preview" {
			return map[string]any{"kind": "image", "mimeType": "image/png", "url": server.URL}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	picture, err := p.answerOptions("lawful-large-image").LoadImage("https://author.example.invalid/large.png")
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := picture.(*ui.Bitmap)
	if !ok {
		t.Fatal("a valid image below Cindy's 100 MiB limit was not decoded natively")
	}
	if w, h := decoded.Size(); w != 160 || h != 96 {
		t.Fatal("the large legal PNG was replaced by a placeholder")
	}
	t.Logf("valid CRC PNG bytes=%d >64 MiB and <100 MiB, actual native pixels 160x96", len(image))
}

package workspace

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

// A lawful private ancillary PNG chunk exercises the byte contract without
// allocating a 65 MiB pixel surface or relying on malformed trailing bytes.
func TestImagePaneKeepsTheOriginal100MiBPreviewBudget(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 160, 96))); err != nil {
		t.Fatal(err)
	}
	const padding = 65 << 20
	raw := encoded.Bytes()
	at := len(raw) - 12
	picture := make([]byte, len(raw)+padding+12)
	copy(picture, raw[:at])
	binary.BigEndian.PutUint32(picture[at:], padding)
	copy(picture[at+4:], "vpAg")
	binary.BigEndian.PutUint32(picture[at+8+padding:], crc32.ChecksumIEEE(picture[at+4:at+8+padding]))
	copy(picture[at+12+padding:], raw[at:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Path == "/over-budget" {
			// Stream through the same real bounded reader. Declared size alone
			// is not evidence of enforcement, nor is a decoder error enough.
			chunk := make([]byte, 8192)
			for remaining := 100<<20 + 1; remaining > 0; {
				n, err := w.Write(chunk[:min(remaining, len(chunk))])
				if err != nil {
					return
				}
				remaining -= n
			}
			return
		}
		_, _ = w.Write(picture)
	}))
	defer server.Close()
	h := testhost.New(t, func(string, json.RawMessage) (any, error) { return nil, fmt.Errorf("unexpected Host operation") })
	pane := NewImagePane(h.Env, FilePreview{Kind: "image", MIME: "image/png", Name: "large.png", URL: server.URL + "/owned"})
	t.Cleanup(pane.Close)
	tt := ui.NewTester(pane.View, 560, 480)
	h.Settle(tt)
	if pane.loading || pane.err != "" || pane.image == nil {
		t.Fatalf("original legal preview rejected: loading=%t error=%q", pane.loading, pane.err)
	}
	if width, height := pane.image.Size(); width != 160 || height != 96 {
		t.Fatal("the native pane substituted a placeholder for the actual PNG")
	}
	pane.Close()
	if !pane.closed || pane.image != nil || pane.video != nil || pane.ctx.Err() == nil {
		t.Fatal("closing the pane retained its decoder or read context")
	}
	data, err := FetchPreview(pane.ctx, server.URL+"/over-budget", 100<<20)
	if err == nil || data != nil {
		t.Fatal("closed preview context still admitted a read")
	}
	over := NewImagePane(h.Env, FilePreview{Kind: "image", MIME: "image/png", Name: "over.png", URL: server.URL + "/over-budget"})
	t.Cleanup(over.Close)
	tt = ui.NewTester(over.View, 560, 480)
	h.Settle(tt)
	if over.loading || over.err != i18n.T("chat.fileReadFailed") || over.image != nil {
		t.Fatal("an oversized preview escaped the bounded reader or hid its failure")
	}
	t.Logf("real native preview decoded %d lawful PNG bytes; >100 MiB rejected and Close released the context", len(picture))
}

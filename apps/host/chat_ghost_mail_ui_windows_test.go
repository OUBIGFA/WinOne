//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Read-only mail uses the original Reader body limit and a real private Bun
// transform, not compose's smaller limit or a JSON-lines sized placeholder.
func TestNativeMailReaderKeepsLargeBodiesAndSanitizesThroughTheOriginalHelpers(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("mail presentation must not call a model")
		http.Error(w, "unexpected", 500)
	}))
	defer model.Close()
	f := newMcpHostFixture(t, model)
	copyOneBotTestRuntime(t, filepath.Join(filepath.Dir(f.worker), "ghost-mail-ui-sdk.js"), filepath.Join(f.bundle, "shared", "chat", "ghost-mail-ui-sdk.js"))
	source := "<!doctype html><html><head><style>p{font-weight:500}</style></head><body><p>" + strings.Repeat("Reader content · ", 48000) + "</p><script>MAIL_SCRIPT_MUST_NOT_RUN</script></body></html>"
	if len(source) <= 1<<19 {
		t.Fatal("fixture must exercise the large Reader contract")
	}
	input, _ := json.Marshal(map[string]any{"op": "document", "reading": true, "source": source})
	raw, err := f.g.mailUi(f.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HTML, Script      string
		HasExternalImages bool
	}
	if json.Unmarshal(raw, &result) != nil || !strings.Contains(result.HTML, "Reader content") || strings.Contains(result.HTML, "MAIL_SCRIPT_MUST_NOT_RUN") || !strings.Contains(result.Script, "WinOneMail") {
		t.Fatal("large Reader document/helper contract was truncated or not sanitized")
	}
	// The same body is not legal as an editable compose form.
	input, _ = json.Marshal(map[string]any{"op": "document", "source": source})
	if _, err = f.g.mailUi(f.ctx, input); err == nil {
		t.Fatal("Reader size exemption expanded compose mutation limits")
	}
	input, _ = json.Marshal(map[string]any{"op": "model", "reading": true, "values": map[string]string{"bodyHtml": source, "to": "Reader <reader@example.invalid>", "attachmentItems": "[]"}})
	if _, err = f.g.mailUi(f.ctx, input); err != nil {
		t.Fatal("read-only Reader form projection lost the original long body", err)
	}
	items, err := os.ReadDir(filepath.Join(f.g.control.directory, "chat", "ghosts", "native-mail-ui"))
	if err != nil || len(items) != 0 {
		t.Fatal("presentation retained request/result temporary files", err)
	}
}

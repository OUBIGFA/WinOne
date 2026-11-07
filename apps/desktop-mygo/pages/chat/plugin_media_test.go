package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"winone/desktopmygo/chat/pluginhost/globalpages"
)

func TestPluginMediaWaitsForActualImportAndPreservesTheNativeDraft(t *testing.T) {
	f := newFakeChat(t)
	root := t.TempDir()
	source, imported := filepath.Join(root, "owned.png"), filepath.Join(root, "imported.png")
	data := pngBytes(t)
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	imports := 0
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.ghosts.resolve-panel-media":
			return map[string]any{"path": source, "name": "Artwork.png", "kind": "image", "mimeType": "image/png", "size": len(data)}, nil
		case "chat.attachments.add":
			var input struct {
				ID    string
				Paths []string
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			if input.ID != "c1" || len(input.Paths) != 1 || input.Paths[0] != source {
				t.Errorf("wrong import owner/path: %+v", input)
			}
			imports++
			bytes, err := os.ReadFile(input.Paths[0])
			if err != nil {
				return nil, err
			}
			if err = os.WriteFile(imported, bytes, 0600); err != nil {
				return nil, err
			}
			return map[string]any{"attachments": []any{map[string]any{"path": imported, "name": "Artwork.png", "mimeType": "image/png", "size": len(bytes)}}}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.updateText("Keep this draft", nil, "c1")
	settle(h, tt)
	p.busy = true
	acks := 0
	globalpages.For(p.env).AttachMedia("cindy-ghost://fixture/media/owned.png", "c1", "fixture", "native-page", "", func(err error) {
		if err != nil {
			t.Error(err)
		}
		acks++
	})
	settle(h, tt)
	if acks != 0 || imports != 0 || len(p.files) != 0 {
		t.Fatal("ledger metadata was reported as an imported attachment")
	}
	p.busy = false
	settle(h, tt)
	if acks != 1 || imports != 1 || p.text != "Keep this draft" || len(p.files) != 1 || p.files[0].Path != imported || len(f.prompts) != 0 {
		t.Fatalf("import changed or sent the draft: acks=%d imports=%d text=%q files=%+v", acks, imports, p.text, p.files)
	}
	got, err := os.ReadFile(imported)
	if err != nil || string(got) != string(data) {
		t.Fatal("actual import bytes were not retained")
	}
}

func TestPluginMediaEmptyImportCannotReportSuccess(t *testing.T) {
	f := newFakeChat(t)
	path := filepath.Join(t.TempDir(), "owned.png")
	if err := os.WriteFile(path, pngBytes(t), 0600); err != nil {
		t.Fatal(err)
	}
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.ghosts.resolve-panel-media" {
			return map[string]any{"path": path, "name": "owned.png", "kind": "image", "mimeType": "image/png"}, nil
		}
		if method == "chat.attachments.add" {
			return map[string]any{"attachments": []any{}}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.text = "Unsent"
	var result error
	globalpages.For(p.env).AttachMedia("cindy-ghost://fixture/media/owned.png", "c1", "fixture", "native-page", "", func(err error) { result = err })
	settle(h, tt)
	if result == nil || len(p.files) != 0 || p.text != "Unsent" || len(f.prompts) != 0 {
		t.Fatal("an empty Host import became a fake success")
	}
}
func TestPluginMediaTransferKeepsOpaqueURIAndSourceTicket(t *testing.T) {
	uri := "cindy-ghost://fixture/media/owned.png"
	data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.URIList, []byte("# comment\r\n"+uri)), transfer.Bytes(pluginHandoverFormat, []byte(`{"uri":"`+uri+`","sourceToken":"native-source"}`))))
	got, token, found, err := pluginMediaSource(data)
	if err != nil || !found || got != uri || token != "native-source" {
		t.Fatalf("drop lost its native provenance: %q %q %v %v", got, token, found, err)
	}
	if got, _, found, err := pluginMediaSource(transfer.New(transfer.NewItem(transfer.Bytes(transfer.URIList, []byte("https://example.com/media/owned.png"))))); err != nil || found || got != "" {
		t.Fatal("an arbitrary web URI became plugin media")
	}
	if _, _, found, err := pluginMediaSource(transfer.New(transfer.NewItem(transfer.Bytes(pluginHandoverFormat, []byte(strings.Repeat("x", 4097)))))); !found || err == nil {
		t.Fatal("malformed native handover silently fell back to files")
	}
}

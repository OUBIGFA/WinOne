package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

// The fixture supplies only the real Host preview boundary. Both persisted
// toolResult parts and live tool events pass through the production projection
// and native document/image controls, never through a synthesized media URL.
func TestNativeToolImagesUseOwnedPreviewsForStoredAndLiveResults(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprint("live=", live), func(t *testing.T) {
			f := newFakeChat(t)
			paths := []string{`C:\fixture\attachments\tool-media\first.png`, `C:\fixture\attachments\tool-media\second.png`}
			parts := []model.Part{{Type: "text", Text: "Actual image result"}, {Type: "image", MimeType: "image/png", Path: paths[0]}, {Type: "image", MimeType: "image/png", Path: paths[1]}}
			f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Inspect images")}, {ID: "a", Role: "assistant", TurnID: "u", Content: model.Content{Parts: []model.Part{{Type: "toolCall", ID: "capture", Name: "screenshot", Arguments: map[string]any{}}}}}}
			if !live {
				f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: "result", Role: "toolResult", ToolCallID: "capture", TurnID: "u", Content: model.Content{Parts: parts}})
			}
			image := pngBytes(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(image)
			}))
			t.Cleanup(server.Close)
			var previews []string
			h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
				if method == "chat.file.preview" {
					var input struct{ ID, Path string }
					_ = json.Unmarshal(raw, &input)
					if input.ID != "c1" || input.Path != paths[0] && input.Path != paths[1] {
						return nil, fmt.Errorf("PREVIEW_DENIED")
					}
					previews = append(previews, input.Path)
					return map[string]any{"path": input.Path, "name": "Result image", "kind": "image", "mimeType": "image/png", "url": server.URL}, nil
				}
				return f.handle(method, raw)
			})
			open(t, h, p, tt, "c1")
			if live {
				encoded, _ := json.Marshal(map[string]any{"content": parts})
				var result map[string]any
				_ = json.Unmarshal(encoded, &result)
				p.view = model.Apply(p.view, model.Event{Type: "pi_event", EventType: "tool_execution_end", ToolCallID: "capture", ToolName: "screenshot", TurnID: "u", Result: result})
			}
			p.ui.expanded = map[string]bool{"process:u": true}
			settle(h, tt)
			if err := tt.Click("screenshot"); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if len(previews) != 2 || !tt.HasText(i18n.T("chat.zoomIn")) || !tt.HasText("Result image") {
				t.Fatalf("tool images did not reach the native preview: paths=%v texts=%q", previews, tt.Texts())
			}
			settle(h, tt)
			if len(previews) != 2 || len(tt.OpenedURLs()) != 0 {
				t.Fatal("a redraw reread images or escaped to the browser")
			}
			p.show("c2")
			settle(h, tt)
		})
	}
}

func TestNativeReadImageUsesResultSnapshotAndCurrentFileRemainsSeparate(t *testing.T) {
	f := newFakeChat(t)
	owned := `C:\fixture\attachments\tool-media\saved.png`
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Read image")}, {ID: "a", Role: "assistant", TurnID: "u", Content: model.Content{Parts: []model.Part{{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": "input.png"}}}}}, {ID: "result", Role: "toolResult", ToolCallID: "read", TurnID: "u", Content: model.Content{Parts: []model.Part{{Type: "image", MimeType: "image/png", Path: owned}}}}}
	var previews []string
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.file.preview" {
			var input struct{ ID, Path string }
			_ = json.Unmarshal(raw, &input)
			previews = append(previews, input.Path)
			return nil, fmt.Errorf("READ_PERMISSION_DENIED")
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.ui.expanded = map[string]bool{"process:u": true}
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.toolVerb_read") + " input.png"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(previews) != 1 || previews[0] != owned || !tt.HasText("READ_PERMISSION_DENIED") || len(tt.OpenedURLs()) != 0 {
		t.Fatalf("image snapshot bypassed its authority: paths=%v texts=%q", previews, tt.Texts())
	}
	if err := tt.Click(i18n.T("chat.openCurrentFile")); err != nil {
		t.Fatal(err)
	}
	if request := p.ui.panelRequest; request == nil || request.File != `E:\WinOne\input.png` {
		t.Fatalf("current-file action lost the original file: %+v", request)
	}
	settle(h, tt)
	if p.ui.items.tool != nil || !strings.HasSuffix(previews[len(previews)-1], "input.png") {
		t.Fatal("opening the current file did not leave the immutable tool snapshot")
	}
}

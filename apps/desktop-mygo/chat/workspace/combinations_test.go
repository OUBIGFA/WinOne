package workspace

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/testhost"
)

// Visibility is a lifecycle contract: a deferred preview must inherit the
// panel's current state instead of waking a hidden player's poll loop.
type visibilityBody struct{ visible, closed bool }

func (b *visibilityBody) View(*ui.Context)  {}
func (b *visibilityBody) SetVisible(v bool) { b.visible = v }
func (b *visibilityBody) Close()            { b.closed = true }

func TestDeferredMediaPreviewInheritsTheHiddenDocumentState(t *testing.T) {
	h := testhost.New(t, func(string, json.RawMessage) (any, error) {
		return FilePreview{Kind: "audio", Name: "silent.wav"}, nil
	})
	body := &visibilityBody{visible: true}
	d := NewDocumentPane(h.Env, DocumentOptions{Prefix: "chat.", SessionID: "owned", Path: "silent.wav", Media: func(FilePreview) (DocumentBody, error) { return body, nil }})
	d.SetVisible(false)
	tt := ui.NewTester(func(*ui.Context) {}, 600, 400)
	h.Settle(tt)
	defer d.Close()
	if body.visible {
		t.Fatal("late media preview became active in a hidden panel")
	}
	d.SetVisible(true)
	if !body.visible {
		t.Fatal("showing the document did not reactivate its body")
	}
}

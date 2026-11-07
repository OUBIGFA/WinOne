package pluginhost

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

const nativeMailKey = "12345678-1234-1234-1234-123456789abc"

func nativeMailPayload(t *testing.T, state string, rich bool) json.RawMessage {
	t.Helper()
	fields := []cardFormField{{Name: "to", Label: "To", Value: "Recipient <recipient@example.invalid>", Kind: "recipients", Row: "recipients"},
		{Name: "cc", Label: "Cc", Value: "", Kind: "recipients", Row: "recipients"}, {Name: "bcc", Label: "Bcc", Value: "", Kind: "recipients", Collapsed: true},
		{Name: "subject", Label: "Subject", Value: "Saved subject"}, {Name: "body", Label: "Message", Value: "Saved body", Multiline: true}}
	if rich {
		fields[4].Kind = "hidden"
		fields = append(fields, cardFormField{Name: "bodyHtml", Label: "Message", Kind: "mail-html", Value: `<style>.signature{color:#234567}</style><p>Saved <b>body</b><img src="cid:image@fixture"></p>`},
			cardFormField{Name: "quoteHtml", Label: "Quoted message", Kind: "hidden", Value: `<blockquote>Quoted body</blockquote>`},
			cardFormField{Name: "attachmentItems", Label: "Attachments", Kind: "mail-attachments", Value: `[{"key":"image","filename":"image.png","mimeType":"image/png","bytes":68},{"key":"notes","filename":"notes.txt","mimeType":"text/plain","bytes":42}]`})
	}
	mail := map[string]any{"accountId": "account", "draftId": "draft", "key": nativeMailKey, "from": "sender@example.invalid", "fields": fields,
		"attachments": []string{"image.png", "notes.txt"}, "state": state, "status": "SDK status", "sendLabel": "SDK send", "fromLabel": "SDK From", "deleteLabel": "SDK delete"}
	change := ""
	if state == "draft" || state == "syncing" || state == "error" {
		change = "mail:change:" + nativeMailKey
	}
	if state == "sent" {
		mail["continueLabel"] = "SDK continue"
	}
	if state == "sending" {
		mail["pendingSend"] = map[string]any{"deadline": time.Now().Add(10 * time.Second).UnixMilli(), "label": "Sending in {seconds}s", "cancelLabel": "SDK Undo", "sendNowLabel": "SDK now"}
	}
	raw, err := json.Marshal(map[string]any{"callId": "mail", "sessionId": "session", "ghostId": "plugin", "html": `<div>Ordinary footer must not render</div><button data-ghost-action="mail:send:` + nativeMailKey + `">SDK send</button>`,
		"viewHtml": `<div>Ordinary footer must not render</div><button data-ghost-action="mail:send:` + nativeMailKey + `">SDK send</button>`, "fields": fields, "formChange": change, "mail": mail})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The pipe is the component boundary only. Actual SDK/provider/serializer
// execution is exercised by mail-backend.test.mjs and mail_native_windows_test.
func nativeMailHarness(t *testing.T, raw json.RawMessage, width int) (*testhost.Harness, *Host, *ui.Tester) {
	t.Helper()
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "chat.ghosts.card-action" {
			return map[string]bool{"ok": true}, nil // Delivery only, never a sent receipt.
		}
		if method == "chat.ghosts.mail-ui" {
			var input struct{ Op string }
			_ = json.Unmarshal(params, &input)
			if input.Op == "model" {
				return map[string]any{"attachments": []map[string]any{{"key": "image", "filename": "image.png", "mimeType": "image/png", "bytes": 68}, {"key": "notes", "filename": "notes.txt", "mimeType": "text/plain", "bytes": 42}}, "insertable": []string{"image"}, "recipients": map[string]any{"to": []map[string]string{{"address": "Recipient <recipient@example.invalid>", "label": "Recipient"}}}}, nil
			}
		}
		return nil, nil
	})
	host := For(harness.Env)
	t.Cleanup(host.Close)
	host.cards.update(raw)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
		ui.Scroll(c).Fill().Children(func() { host.Card(c, "mail") })
	}, width, 1600)
	harness.Settle(tt)
	return harness, host, tt
}

func TestNativeMailConsumesHostPayloadNotOrdinaryHTMLButtons(t *testing.T) {
	harness, host, tt := nativeMailHarness(t, nativeMailPayload(t, "draft", false), 560)
	if tt.HasText("Ordinary footer must not render") || !tt.HasText("SDK status") || !tt.HasText("SDK From: sender@example.invalid") || !tt.HasText("SDK delete") {
		t.Fatalf("mail contract was not rendered independently: %v", tt.Texts())
	}
	if err := tt.Click("Subject"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Locally authored subject")
	harness.Settle(tt)
	if err := tt.Click("SDK send"); err != nil {
		t.Fatal(err)
	}
	harness.Settle(tt)
	calls := harness.Called("chat.ghosts.card-action")
	var submit struct {
		Action     string
		FormValues map[string]string
	}
	_ = json.Unmarshal(calls[len(calls)-1].Params, &submit)
	if submit.Action != "mail:send:"+nativeMailKey || submit.FormValues["subject"] != "Locally authored subject" || submit.FormValues["body"] != "Saved body" || len(submit.FormValues) != 5 {
		t.Fatalf("mail send omitted original complete form: %+v", submit)
	}
	if host.cards.byCall["mail"].Mail.State != "draft" {
		t.Fatal("dispatcher acknowledgement was shown as provider send success")
	}
}

func TestNativeMailRichFieldsNeverExposeRawMarkupAndKeepAllAttachmentActions(t *testing.T) {
	harness, _, tt := nativeMailHarness(t, nativeMailPayload(t, "draft", true), 560)
	joined := strings.Join(tt.Texts(), " ")
	if strings.Contains(joined, "<style>") || strings.Contains(joined, "attachmentItems") || strings.Contains(joined, `[{"key"`) {
		t.Fatalf("native mail drew source/JSON rather than real editor and attachments: %s", joined)
	}
	for _, label := range []string{"image.png", "notes.txt", mailText("attach"), mailText("insertImage"), mailText("remove") + ": notes.txt"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("mail omitted %q: %v", label, tt.Texts())
		}
	}
	if err := tt.Click(mailText("attach")); err != nil {
		t.Fatal(err)
	}
	harness.Settle(tt)
	calls := harness.Called("chat.ghosts.card-action")
	if len(calls) != 0 {
		t.Fatal("a missing native HTML editor was treated as a successful attachment selection")
	}
	if !tt.HasText(mailText("syncFailed")) {
		t.Fatalf("missing editor did not report its delivery barrier: %v", tt.Texts())
	}
}

func TestNativeMailReadonlyStatesAndHistoryCannotAutosaveOrSend(t *testing.T) {
	for _, state := range []string{"sending", "sent", "unconfirmed", "deleting", "deleted"} {
		t.Run(state, func(t *testing.T) {
			harness, host, tt := nativeMailHarness(t, nativeMailPayload(t, state, false), 560)
			item := host.cards.byCall["mail"]
			item.formDirty = true
			harness.Settle(tt)
			if len(harness.Called("chat.ghosts.card-action")) != 0 {
				t.Fatal("readonly mail emitted an automatic form event")
			}
			if tt.HasText("SDK send") || tt.HasText("SDK delete") {
				t.Fatalf("readonly mail offered editing operations: %v", tt.Texts())
			}
			if state == "sending" && (!tt.HasText("SDK Undo") || !tt.HasText("SDK now")) {
				t.Fatal("countdown lost Undo/Send now")
			}
			if state == "sent" && !tt.HasText("SDK continue") {
				t.Fatal("known sent receipt lost Continue")
			}
		})
	}
	raw := nativeMailPayload(t, "draft", false)
	var row map[string]any
	_ = json.Unmarshal(raw, &row)
	row["mail"].(map[string]any)["supersededBy"] = "new-mail"
	raw, _ = json.Marshal(row)
	harness, host, tt := nativeMailHarness(t, raw, 560)
	host.cards.byCall["mail"].formDirty = true
	harness.Settle(tt)
	if tt.HasText("SDK send") || tt.HasText("SDK delete") || len(harness.Called("chat.ghosts.card-action")) > 0 {
		t.Fatal("superseded snapshot remained another composer")
	}
	if !tt.HasText(mailText("updated")) {
		t.Fatal("superseded snapshot lost readable history entry")
	}
}

func TestNativeMailChangingIdentityRetiresOldLocalValuesAndInFlightCallbacks(t *testing.T) {
	harness, host, tt := nativeMailHarness(t, nativeMailPayload(t, "draft", false), 560)
	item := host.cards.byCall["mail"]
	item.values["subject"] = "Old draft typing"
	var update map[string]any
	_ = json.Unmarshal(nativeMailPayload(t, "draft", false), &update)
	update["mail"].(map[string]any)["key"] = "87654321-4321-4321-4321-cba987654321"
	update["formChange"] = "mail:change:87654321-4321-4321-4321-cba987654321"
	raw, _ := json.Marshal(update)
	host.cards.update(raw)
	harness.Settle(tt)
	if item.values["subject"] != "Saved subject" || item.formDirty {
		t.Fatal("a new logical draft key inherited old editor state")
	}
	item.values["subject"] = "New local typing"
	host.cards.update(raw)
	if item.values["subject"] != "New local typing" {
		t.Fatal("same draft echo erased newer local typing")
	}
}

func TestNativeMailFooterIsAccessibleAndFitsBothLanguagesAtNarrowWidth(t *testing.T) {
	previous := i18n.Locale()
	defer i18n.SetLocale(previous)
	for _, language := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(language)
		_, _, tt := nativeMailHarness(t, nativeMailPayload(t, "sending", false), 320)
		for _, label := range []string{"SDK Undo", "SDK now"} {
			rect, ok := tt.Find(label)
			if !ok || rect.X < 0 || rect.X+rect.W > 321 {
				t.Fatalf("mail action %s overflowed narrow %s: %+v", label, language, rect)
			}
		}
	}
}

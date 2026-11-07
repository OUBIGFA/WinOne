package chat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/i18n"
)

func TestNativeMailLinkUsesReadFalseProbeAndReaderRootOrBrowserFallback(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprint(supported), func(t *testing.T) {
			f := newFakeChat(t)
			var probes []map[string]any
			h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
				if method == "chat.ghosts.mail-preview" {
					var input map[string]any
					_ = json.Unmarshal(raw, &input)
					probes = append(probes, input)
					if supported && input["read"] == true {
						return map[string]any{"supported": true, "providerId": "fixture", "message": map[string]any{"accountId": "fixture-account", "messageId": "message", "subject": "Native reader through Host.View", "bodyText": "Read-only original mail"}}, nil
					}
					return map[string]bool{"supported": supported}, nil
				}
				if method == "chat.ghosts.mail-ui" {
					return map[string]any{"attachments": []any{}, "images": []any{}, "recipients": map[string]any{}}, nil
				}
				if method == "chat.ghosts.pending" {
					return map[string]any{}, nil
				}
				return f.handle(method, raw)
			})
			open(t, h, p, tt, "c1")
			p.openURL("https://example.invalid/mail")
			settle(h, tt)
			if len(probes) == 0 || probes[0]["sessionId"] != "c1" || probes[0]["read"] != false {
				t.Fatalf("normal link did not use read:false captured owner: %v", probes)
			}
			if !supported {
				if len(probes) != 1 || p.ui.panelRequest == nil || p.ui.panelRequest.URL != "https://example.invalid/mail" {
					t.Fatal("unsupported mail did not use the original browser helper")
				}
				return
			}
			if len(probes) != 2 || probes[1]["read"] != true || probes[1]["opened"] != true || p.ui.panelRequest != nil {
				t.Fatalf("supported mail bypassed the actual reader: %v", probes)
			}
			root := ui.NewTester(func(c *ui.Context) { pluginhost.For(p.env).View(c) }, 1000, 800)
			h.Settle(root)
			if !root.HasText("Native reader through Host.View") || !root.HasText("Read-only original mail") {
				t.Fatal("Root Host.View did not render the reader")
			}
			if err := root.Click("在邮箱中打开"); err != nil {
				t.Fatal(err)
			}
			h.Settle(root)
			h.Settle(tt)
			if len(probes) != 2 || p.ui.panelRequest == nil || p.ui.panelRequest.URL != "https://example.invalid/mail" {
				t.Fatal("reader mailbox action recursively reopened mail or lost the browser link")
			}
		})
	}
}

func TestNativeReaderPreviewBypassDoesNotProbeAgain(t *testing.T) {
	f := newFakeChat(t)
	probes := 0
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.ghosts.mail-preview" {
			probes++
			return map[string]bool{"supported": true}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	pluginhost.For(p.env).QueueMailBrowser("c1", "https://example.invalid/mailbox")
	h.Settle(tt)
	if probes != 0 || p.ui.panelRequest == nil || p.ui.panelRequest.URL != "https://example.invalid/mailbox" {
		t.Fatal("mailReaderBypass fell back into mail probe")
	}
}

func TestNativeMailFailedDeliveryBlocksSendSwitchAndCloseEvenAfterErrorDismiss(t *testing.T) {
	f := newFakeChat(t)
	fail := true
	h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.ghosts.card-get":
			return json.RawMessage(`{"callId":"mail","sessionId":"c1","html":"<div>Original mail</div>","formChange":"mail:change:edit","fields":[{"name":"subject","label":"Subject","value":"Original"}],"mail":{"accountId":"account","draftId":"draft","key":"edit","state":"draft","fields":[{"name":"subject","label":"Subject","value":"Original"}],"sendLabel":"Send mail"}}`), nil
		case "chat.ghosts.mail-ui":
			return map[string]any{"attachments": []any{}, "images": []any{}, "recipients": map[string]any{}}, nil
		case "chat.ghosts.card-action":
			if fail {
				return nil, fmt.Errorf("MAIL_DELIVERY_FAILED")
			}
			return map[string]bool{"ok": true}, nil
		}
		return f.handle(method, raw)
	})
	open(t, h, p, tt, "c1")
	p.updateText("Keep the owner draft", nil, "c1")
	h.Settle(tt)
	cards := ui.NewTester(func(c *ui.Context) { pluginhost.For(p.env).Card(c, "mail") }, 700, 400)
	h.Settle(cards)
	focusNativeInput(t, cards, "Subject")
	cards.Key(ui.Ctrl, ui.KeyA)
	cards.Type("Keep unsynchronized mail")
	h.Settle(cards)
	if !cards.HasText("MAIL_DELIVERY_FAILED") {
		t.Fatal("actual failed form delivery did not surface")
	}
	var sendErr, switchErr error
	p.send(nil, "", func(err error) { sendErr = err })
	p.choose("c2", nil, func(err error) { switchErr = err })
	p.Close()
	h.Settle(cards)
	if sendErr == nil || switchErr == nil || p.closed() || p.activeID != "c1" || p.text != "Keep the owner draft" || len(f.prompts) != 0 {
		t.Fatal("failed mail delivery sent/navigated/closed or discarded edits")
	}
	if err := cards.Click(i18n.T("notices.dismiss")); err != nil {
		t.Fatal(err)
	}
	h.Settle(cards)
	p.Close()
	if p.closed() {
		t.Fatal("dismissing the error cleared the underlying mail barrier")
	}
	fail = false
	if err := cards.Click(i18n.T("pluginHost.retry")); err != nil {
		t.Fatal(err)
	}
	h.Settle(cards)
	p.Close()
	h.Settle(cards)
	if !p.closed() {
		t.Fatal("successful real form delivery could not finish Close")
	}
}

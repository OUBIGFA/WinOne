package pluginhost

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestNativeMailSessionBarrierAndFailedDeliveryKeepTheOriginalEdits(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "chat.ghosts.card-action" {
			close(started)
			<-release
			return nil, testhost.Error{Code: "failed", Message: "Actual delivery unavailable"}
		}
		return nil, nil
	})
	host := For(harness.Env)
	defer host.Close()
	var other map[string]any
	_ = json.Unmarshal(nativeMailPayload(t, "draft", false), &other)
	other["callId"] = "other"
	raw, _ := json.Marshal(other)
	host.cards.update(raw)
	item := host.cards.byCall["other"]
	item.values["subject"] = "Typing in another mail view"
	item.formDirty = true
	finished, accepted := false, true
	host.FlushMailEdits("session", func(ok bool) { finished, accepted = true, ok })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("same-task editor delivery did not start")
	}
	if finished {
		t.Fatal("Chat send overtook the actual pending card delivery")
	}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); host.mailCard(c, item, false) }, 560, 900)
	tt.Frame()
	_, busy := harness.Env.Activity.State()
	if !busy {
		t.Fatal("pending mail delivery did not protect native navigation")
	}
	close(release)
	harness.Settle(tt)
	if !finished || accepted || item.values["subject"] != "Typing in another mail view" || !item.formDirty {
		t.Fatal("failed delivery erased typing or reported a successful barrier")
	}
	dirty, busy := harness.Env.Activity.State()
	if !dirty || busy {
		t.Fatal("failed mail edits lost unsaved-change protection")
	}
}

func TestNativeMailNewEditorIdentityInvalidatesTheOldDeliveryCallback(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "chat.ghosts.card-action" {
			close(started)
			<-release
			return nil, testhost.Error{Code: "old", Message: "Old key failure"}
		}
		return nil, nil
	})
	host := For(harness.Env)
	defer host.Close()
	host.cards.update(nativeMailPayload(t, "draft", false))
	item := host.cards.byCall["mail"]
	item.values["subject"] = "Old local edit"
	item.formDirty = true
	settled, accepted := false, true
	host.FlushMailEdits("session", func(ok bool) { settled, accepted = true, ok })
	<-started
	var update map[string]any
	_ = json.Unmarshal(nativeMailPayload(t, "draft", false), &update)
	update["mail"].(map[string]any)["key"] = "87654321-4321-4321-4321-cba987654321"
	update["formChange"] = "mail:change:87654321-4321-4321-4321-cba987654321"
	raw, _ := json.Marshal(update)
	host.cards.update(raw)
	if !settled || accepted {
		t.Fatal("retired editor retained the old successful delivery barrier")
	}
	close(release)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); host.mailCard(c, item, false) }, 560, 900)
	harness.Settle(tt)
	if item.err != "" || item.values["subject"] != "Saved subject" || item.formDirty || item.formSending {
		t.Fatal("old callback modified the new key's values/error/busy state")
	}
}

// Real production Collapsible, Reader request flags and native pipe. The fake
// boundary supplies data only; actual provider/SDK behavior has separate real
// backend and WebView2 tests, so this cannot claim a mailbox was marked read.
func TestNativeMailReaderManualExpansionCarriesOpenedAndKeepsBcc(t *testing.T) {
	var calls []struct {
		URL    string
		Opened bool
	}
	message := func(id string) *mailMessage {
		return &mailMessage{AccountID: "account", MessageID: id, From: "Sender", To: "to@example.invalid", Bcc: "private@example.invalid", Date: "2026-11-06", BodyText: "Read-only body", Attachments: []mailAttachment{}}
	}
	harness := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.ghosts.mail-preview" {
			var input struct {
				URL    string
				Opened bool
			}
			_ = json.Unmarshal(raw, &input)
			calls = append(calls, input)
			id := strings.TrimPrefix(input.URL, "https://mail.example.invalid/")
			result := mailPreviewResult{Supported: true, ProviderID: "provider", Message: message(id)}
			if id == "selected" {
				result.Message.Conversation = &mailConversation{ThreadID: "thread"}
				for i := range 8 {
					key := "older-" + itoa(i)
					from := "Sender " + itoa(i)
					if i == 0 {
						from = "Earlier"
					}
					if i == 4 {
						key = "selected"
					}
					result.Message.Conversation.Messages = append(result.Message.Conversation.Messages, mailMessageSummary{MessageID: key, URL: "https://mail.example.invalid/" + key, From: from, Date: "2026-11-06"})
				}
			}
			return result, nil
		}
		if method == "chat.ghosts.mail-ui" {
			return map[string]any{"attachments": []any{}, "images": []any{}, "insertable": []any{}, "recipients": map[string]any{}}, nil
		}
		return nil, nil
	})
	host := For(harness.Env)
	defer host.Close()
	host.OpenMail("session", "https://mail.example.invalid/selected")
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); host.MailView(c) }, 1100, 1600)
	harness.Settle(tt)
	if !tt.HasText("private@example.invalid") {
		t.Fatal("Reader hid Bcc behind the compose summary")
	}
	for _, call := range calls {
		if call.URL != "https://mail.example.invalid/selected" && call.Opened {
			t.Fatal("background thread body fetch marked a hidden message open")
		}
	}
	if err := tt.Click("Earlier · 2026-11-06"); err != nil {
		t.Fatal(err)
	}
	harness.Settle(tt)
	opened := 0
	for _, call := range calls {
		if call.URL == "https://mail.example.invalid/older-0" && call.Opened {
			opened++
		}
	}
	if opened != 1 {
		t.Fatalf("manual expansion should carry exactly one original opened flag, got %d (%+v)", opened, calls)
	}
}

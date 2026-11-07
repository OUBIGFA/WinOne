package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

const nativeRichForm = "```cindy-ui\n" +
	`{"version":1,"seq":1,"op":"create","id":"name","kind":"input","props":{"label":"Name","default":"Ada"}}` + "\n" +
	`{"seq":2,"op":"create","id":"approved","kind":"check","props":{"label":"Approve","default":false}}` + "\n" +
	`{"seq":3,"op":"create","id":"days","kind":"range","props":{"label":"Days","min":1,"max":10,"default":3}}` + "\n" +
	`{"seq":4,"op":"create","id":"format","kind":"select","props":{"label":"Format","default":"text","options":[{"value":"text","label":"Text"},{"value":"table","label":"Table"}]}}` + "\n" +
	`{"seq":5,"op":"create","id":"more","kind":"details","children":["note"],"props":{"label":"More"}}` + "\n" +
	`{"seq":6,"op":"create","id":"note","parent":"more","kind":"text","props":{"text":"Full details"}}` + "\n" +
	`{"seq":7,"op":"create","id":"send","kind":"button","props":{"label":"Send form","send":{"op":"concat","args":["Name: ",{"ref":"name"},"; days: ",{"ref":"days"}]}}}` + "\n```"

func TestNativeRichFormUsesControlsAndBusySendCannotGetStuck(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", Content: model.TextContent("Make a form")}, {ID: "a", Role: "assistant", TurnID: "u", Content: model.TextContent(nativeRichForm)}}
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	focusNativeInput(t, tt, "Name")
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Grace")
	if err := tt.Click("Approve"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("Full details") || !tt.HasText("Name: Grace; days: 3") {
		t.Fatalf("native form values/details did not render: %q", tt.Texts())
	}
	p.busy = true
	settle(h, tt)
	if err := tt.Click("Send form"); err == nil {
		settle(h, tt)
	}
	if len(f.prompts) != 0 || len(p.richMemory.sending) != 0 {
		t.Fatal("busy form send installed a spinner without a real submission")
	}
	p.busy = false
	p.view.Messages = append(p.view.Messages, model.Message{ID:"continuing", Role:"assistant", TurnID:"u", Content:model.Content{Parts:[]model.Part{{Type:"thinking", Thinking:"Continue after the delivered form"}}}})
	p.view.Running = true
	settle(h, tt)
	if err := tt.Click("Send form"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.prompts) != 1 || f.prompts[0]["deliveryMode"] != "queue" || p.awaitingInput != "" {
		t.Fatalf("running form send did not preserve queue semantics: %v waiting=%q", f.prompts, p.awaitingInput)
	}
	found := false
	for _, message := range p.view.Messages {
		found = found || message.Pending && message.Queued && message.Text() == "Name: Grace; days: 3"
	}
	if !found {
		t.Fatal("accepted form input was not presented as a queued owner message")
	}
}

func TestNativeInlineMediaPreflightIsSingleAndDropsAClosedConversation(t *testing.T) {
	f := newFakeChat(t)
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	// A real preview authority failure must stay on the media control rather
	// than silently opening another player/browser. It must also be retryable.
	calls := 0
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.file.preview" {
			calls++
			return nil, fmt.Errorf("PREVIEW_DENIED")
		}
		return f.handle(method, raw)
	})
	p.playRichMedia("inline", "relative/silent.mp4", "Video")
	p.playRichMedia("inline", "relative/silent.mp4", "Video")
	settle(h, tt)
	if calls != 1 || p.richOpening["inline"] || !strings.Contains(p.ui.items.errors["media:inline"], "PREVIEW_DENIED") || len(tt.OpenedURLs()) != 0 {
		t.Fatal("media preflight duplicated or hid the actual failure")
	}
	p.playRichMedia("inline", "relative/silent.mp4", "Video")
	settle(h, tt)
	if calls != 2 {
		t.Fatal("failed media preflight could not retry")
	}
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.file.preview" {
			return map[string]string{"kind": "video", "url": f.previewURL, "name": "Late video"}, nil
		}
		return f.handle(method, raw)
	})
	p.playRichMedia("late", "relative/silent.mp4", "Late video")
	p.show("c2")
	settle(h, tt)
	if len(p.richMedia) != 0 || len(p.richOpening) != 0 {
		t.Fatal("a late media preview launched a helper for the conversation that was left")
	}
}

// Only component images are claimed here. Real native-window/Host evidence is
// produced by tests/native-ui-matrix; physical IME/audio is explicitly separate.
func TestNativeInteractionShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	defer func() { dark = false; i18n.SetLocale(i18n.Chinese) }()
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, theme := range []bool{false, true} {
			for _, width := range []int{1100, 560} {
				dark = theme
				f := newFakeChat(t)
				f.snapshot.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Hi")}}
				for i := range 8 {
					stamp := float64(100 + i)
					f.snapshot.Messages = append(f.snapshot.Messages, model.Message{ID: fmt.Sprint("a", i), Role: "assistant", TurnID: "u", Timestamp: &stamp, Content: model.Content{Parts: []model.Part{{Type: "toolCall", ID: fmt.Sprint("t", i), Name: "bash", Arguments: map[string]any{"command": fmt.Sprint("step-", i)}}}}})
				}
				h, p, tt := setupWith(t, f, func(method string, raw json.RawMessage) (any, error) {
					if method == "chat.review.turn" {
						return map[string]any{}, nil
					}
					return f.handle(method, raw)
				})
				i18n.SetLocale(locale)
				tt.SetSize(width, 900)
				open(t, h, p, tt, "c1")
				p.view.Running = true
				settle(h, tt)
				name := fmt.Sprintf("%s-%t-%d", locale, theme, width)
				shot(t, tt, "native-preview-"+name)
				stamp := float64(200)
				p.view.Messages = append(p.view.Messages, model.Message{ID: "delivery", Role: "assistant", TurnID: "u", Timestamp: &stamp, Content: model.TextContent("Public prose stays visible before the next tool.")}, model.Message{ID: "continuing", Role: "assistant", TurnID: "u", Content: model.Content{Parts: []model.Part{{Type: "thinking", Thinking: "Continue verifying the delivered result."}}}})
				settle(h, tt)
				shot(t, tt, "native-public-"+name)
				p.beginEdit(p.view.Messages[0])
				settle(h, tt)
				shot(t, tt, "native-edit-"+name)
				p.ui.edit = nil
				for i := range 4 {
					id := fmt.Sprint("q", i)
					p.view.Messages = append(p.view.Messages, model.Message{ID: id, RequestID: id, Role: "user", Pending: true, Queued: true, Content: model.TextContent(fmt.Sprint("Queued owner input ", i))})
				}
				settle(h, tt)
				shot(t, tt, "native-queue-"+name)
				p.view.Messages = []model.Message{{ID: "u", Role: "user", TurnID: "u", Content: model.TextContent("Make a form")}, {ID: "rich", Role: "assistant", TurnID: "u", Content: model.TextContent(nativeRichForm)}}
				p.view.Running = false
				settle(h, tt)
				shot(t, tt, "native-rich-"+name)
				input, ok := tt.Find(i18n.T("chat.message"))
				if !ok || input.X < 0 || input.X+input.W > float32(width)+1 {
					t.Fatalf("composer escaped the matrix window: %+v", input)
				}
				p.Close()
				settle(h, tt)
			}
		}
	}
}

package onebot

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

func TestNativeTopicKeepsBothComposersUsableInANarrowWindow(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
	h, p, tt := setup(t, f)
	p.openBot("self")
	for range 4 {
		h.Settle(tt)
	}
	p.openThread("self", "main", "root", "Topic")
	for range 4 {
		h.Settle(tt)
	}
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, width := range []int{1100, 560, 1100} {
			tt.SetSize(width, 900)
			for range 4 {
				h.Settle(tt)
			}
			input, ok := tt.Find(i18n.T("chat.message"))
			if !ok || input.W < 250 || input.X < 0 || input.X+input.W > float32(width)+1 {
				t.Fatalf("%s width=%d topic made the main input unusable: %+v", locale, width, input)
			}
			if p.topic == nil || p.topicSession != "topic" {
				t.Fatal("responsive topic layout lost its owning controller")
			}
		}
	}
}

func TestNativeGroupSendFailureKeepsSubmittedAndNewerDrafts(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴"), oneBot("peer", "同事")}}
	h, p, _ := setup(t, f)
	started, release := make(chan struct{}), make(chan struct{})
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.onebot.groups.get" {
			return map[string]any{"group": GroupDetail{GroupSummary: GroupSummary{ID: "group", Name: "Group", Members: []GroupMember{{BotID: "self", Name: "小柴"}, {BotID: "peer", Name: "同事"}}}}}, nil
		}
		if method == "chat.onebot.groups.send" {
			close(started)
			<-release
			return nil, errors.New("SEND_FAILED")
		}
		return f.handle(method, raw)
	})
	g := newGroupPane(p, "group")
	defer g.Close()
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); g.View(c) }, 900, 800)
	h.Settle(tt)
	g.draft = groupDraft{Text: "Submitted input", Attachments: []GroupAttachment{{ID: "one", Path: "one.txt", Name: "one.txt"}}}
	g.send(false)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("group send did not start")
	}
	if err := tt.Click(nt("groupMessage")); err != nil {
		t.Fatal(err)
	}
	tt.Type("Newer input")
	close(release)
	h.Settle(tt)
	if g.draft.Text != "Submitted input\n\nNewer input" || len(g.draft.Attachments) != 1 || g.error == "" {
		t.Fatalf("failed group input was silently lost: %+v error=%q", g.draft, g.error)
	}
}

func TestNativeGroupRetryIdentityIncludesMentionsAndDivision(t *testing.T) {
	f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴"), oneBot("peer", "同事")}}
	h, p, _ := setup(t, f)
	var sent []map[string]any
	h.SetHandler(func(method string, raw json.RawMessage) (any, error) {
		if method == "chat.onebot.groups.get" {
			return map[string]any{"group": GroupDetail{GroupSummary: GroupSummary{ID: "group", Name: "Group", Members: []GroupMember{{BotID: "self", Name: "小柴"}, {BotID: "peer", Name: "同事"}}}}}, nil
		}
		if method == "chat.onebot.groups.send" {
			var request struct {
				Input map[string]any `json:"input"`
			}
			_ = json.Unmarshal(raw, &request)
			sent = append(sent, request.Input)
			return nil, errors.New("ACKNOWLEDGEMENT_LOST")
		}
		return f.handle(method, raw)
	})
	g := newGroupPane(p, "group")
	defer g.Close()
	tt := ui.NewTester(g.View, 900, 800)
	h.Settle(tt)
	g.draft.Text = "Same words"
	g.send(false)
	h.Settle(tt)
	g.send(false)
	h.Settle(tt)
	if len(sent) != 2 || sent[0]["clientId"] != sent[1]["clientId"] {
		t.Fatal("an unchanged lost acknowledgement did not retain its receipt")
	}
	g.draft.Mentioned = []string{"peer"}
	g.send(false)
	h.Settle(tt)
	if sent[2]["clientId"] == sent[1]["clientId"] {
		t.Fatal("new mention recipients borrowed an old receipt")
	}
	g.send(true)
	h.Settle(tt)
	if sent[3]["clientId"] == sent[2]["clientId"] || !strings.Contains(g.error, "ACKNOWLEDGEMENT_LOST") {
		t.Fatal("division work reused a conversational receipt")
	}
}

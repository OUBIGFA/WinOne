package chat

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

func TestMessageActionsRevealWithoutMovingContent(t *testing.T) {
	_, h, p, _, _ := setup(t)
	for _, dark := range []bool{false, true} {
		for _, width := range []int{560, 1000} {
			var user ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(kit.Theme(c.Preferences(), dark))
				ui.Column(c).Fill().Padding(24).Gap(20).Background(c.Theme().Background).Children(func() {
					p.userMessage(c, model.Message{ID: "u", Role: "user", Content: model.TextContent("A short saved question")}, true)
					p.replyView(c, model.Message{Role: "assistant", Content: model.TextContent("A readable saved answer.")}, false, "answer", false, false)
				})
			}, width, 350)
			h.Settle(tt)
			user, _ = tt.Find(i18n.T("chat.editMessage"))
			if user.W != 24 || user.H != 24 {
				t.Fatalf("oversized message action: %+v", user)
			}
			pixels := func() []byte {
				var out []byte
				img := tt.Image()
				for y := int(user.Y); y < int(user.Y+user.H); y++ {
					for x := int(user.X); x < int(user.X+user.W); x++ {
						v := img.RGBAAt(x, y)
						out = append(out, v.R, v.G, v.B, v.A)
					}
				}
				return out
			}
			hidden := pixels()
			shot(t, tt, fmt.Sprintf("message-actions-idle-%t-%d", dark, width))
			text, _ := tt.Find("A short saved question")
			tt.Move(text.X+5, text.Y+5)
			h.Settle(tt)
			if bytes.Equal(hidden, pixels()) {
				t.Fatal("hovering the message did not reveal its actions")
			}
			if after, _ := tt.Find(i18n.T("chat.editMessage")); after != user {
				t.Fatal("revealing actions moved the row")
			}
			shot(t, tt, fmt.Sprintf("message-actions-hover-%t-%d", dark, width))
			tt.Move(1, 1)
			h.Settle(tt)
			if !bytes.Equal(hidden, pixels()) {
				t.Fatal("user actions remained visible after leaving the message")
			}
			tt.Key(0, ui.KeyTab)
			h.Settle(tt)
			if bytes.Equal(hidden, pixels()) {
				t.Fatal("keyboard focus did not reveal message actions")
			}
		}
	}
}

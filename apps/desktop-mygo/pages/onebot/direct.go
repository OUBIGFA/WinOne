package onebot

import (
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type DirectMessage struct {
	ID, SenderBotID, SenderBotName, RecipientBotID, RecipientBotName, Content string
	CreatedAt                                                                 int64
	Sequence                                                                  int
}
type DirectThread struct {
	ID, BotAID, BotAName, BotBID, BotBName, Status, CloseReason string
	Messages                                                    []DirectMessage
}
type directPane struct {
	parent                 *page
	threadID, botID        string
	data                   DirectThread
	loading, closed, again bool
	error                  string
	generation             uint64
}

func newDirectPane(parent *page, botID, threadID string) *directPane {
	p := &directPane{parent: parent, botID: botID, threadID: threadID}
	p.Refresh()
	return p
}
func (p *directPane) Close() { p.closed = true; p.generation++ }
func (p *directPane) Refresh() {
	if p.closed {
		return
	}
	if p.loading {
		p.again = true
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	oneBotCall(p.parent, "directMessage.get", map[string]string{"threadId": p.threadID, "viewerBotId": p.botID}, func(value struct {
		Thread DirectThread `json:"thread"`
	}, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.data = value.Thread
		p.error = ""
		if p.again {
			p.again = false
			p.Refresh()
		}
	})
}
func (p *directPane) View(c *ui.Context) {
	ui.Column(c).Fill().Gap(10).Padding(16).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			kit.Strong(c, p.data.BotAName+" ↔ "+p.data.BotBName).Grow(1).MinWidth(0)
			if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
				p.Refresh()
			}
		})
		if p.loading {
			kit.Loader(c)
		}
		kit.InlineError(c, p.error)
		kit.Notice(c, kit.Info, "", nt("directReadOnly"), nil)
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Gap(12).Children(func() {
			for _, message := range p.data.Messages {
				ui.Column(c.Key(message.ID)).FillWidth().Gap(4).Children(func() {
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						if bot := p.parent.bot(message.SenderBotID); bot != nil {
							p.parent.avatars.View(c, *bot, 28)
						}
						kit.Caption(c, message.SenderBotName+" · "+time.UnixMilli(message.CreatedAt).Format("01-02 15:04"))
						ui.Spacer(c)
						if kit.IconButton(c, icons.Copy, nt("copy")).Clicked() {
							c.Services().WriteClipboard(message.Content)
						}
					})
					options := markdown.Options{OnLink: p.parent.openExternal}
					markdown.View(c, message.Content, mathview.For(p.parent.env).Apply(&options)).FillWidth()
				})
			}
		})
		if p.data.CloseReason != "" {
			kit.Status(c, nt("directClose."+p.data.CloseReason))
		}
	})
}
func (p *page) openDirect(botID, threadID string) {
	p.env.RequestChange(func() {
		if p.settings != nil {
			p.settings.Close()
			p.settings = nil
		}
		p.closeContent()
		p.section = "direct"
		p.direct = newDirectPane(p, botID, threadID)
	})
}
func (p *page) openExternal(address string) {
	core.Do(p.env, func() error { return p.env.Host.CallInto("chat.link.open", map[string]string{"url": address}, nil) }, func(err error) {
		if !p.closed && err != nil {
			p.error = err.Error()
		}
	})
}
func (p *page) openTask(sessionID string) {
	for _, bot := range p.profiles {
		for _, session := range bot.Sessions {
			if session.ID != sessionID {
				continue
			}
			if session.Role != "delegation" && session.Kind != "worker" {
				p.openSession(bot.ID, sessionID)
				return
			}
		}
	}
	p.env.Open("chat", core.Intent{Conversation: sessionID, Requested: true})
}
func (p *page) openSessionAt(botID, sessionID, clientID, messageID string) {
	p.env.RequestChange(func() {
		if p.settings != nil {
			p.settings.Close()
			p.settings = nil
		}
		p.showConversation(botID, sessionID)
		p.chat.JumpToMessage(clientID, messageID)
	})
}

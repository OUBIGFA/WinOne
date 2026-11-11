package onebot

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type IMConnection struct {
	ID, BotID, Channel, Service, BotContextID string
	Status                                    struct{ Kind, Reason string }
	Identity                                  struct{ Username, OwnerUserID string }
}
type IMLane struct {
	SessionID, Kind, ChatID, ThreadID, Name, State string
	ContextMessageCount                            int
}
type IMGroup struct{ ChatID, ChatName, Activation string }
type WechatState struct {
	Phase       string
	Bound       bool
	QueuedTasks int
	ErrorCode   string
}
type WechatSettings struct {
	WorkingDir          *string
	WorkingDirAvailable bool
	CompanionBotID      string
}
type channelsPane struct {
	settings                                                                  *profileSettings
	rows                                                                      []IMConnection
	adoptable                                                                 []struct{ Channel, Identity, Service string }
	lanes                                                                     []IMLane
	groups                                                                    []IMGroup
	selected                                                                  string
	adding                                                                    bool
	channel, service, token, ownerID, appID, appSecret, platformBotID, appKey string
	loading, busy, closed, stale, showSecret                                  bool
	error                                                                     string
	remove                                                                    *IMConnection
	adoptChannel, confirmWechat                                               string
	wechat                                                                    WechatState
	wechatSettings                                                            WechatSettings
}

func newChannelsPane(d *profileSettings) *channelsPane {
	p := &channelsPane{settings: d, channel: "telegram", service: "feishu"}
	p.Refresh()
	return p
}
func (p *channelsPane) Dirty() bool {
	return p.adding && (p.token != "" || p.ownerID != "" || p.appID != "" || p.appSecret != "" || p.platformBotID != "" || p.appKey != "")
}
func (p *channelsPane) Busy() bool { return p.busy }
func (p *channelsPane) Discard() {
	p.adding = false
	p.token, p.ownerID, p.appID, p.appSecret, p.platformBotID, p.appKey = "", "", "", "", "", ""
	p.error = ""
}
func (p *channelsPane) Close() { p.closed = true; p.Discard() }
func (p *channelsPane) Invalidate(kind string) {
	if kind == "im-changed" || kind == "wechat-state" || kind == "wechat-settings" {
		p.stale = true
	}
}
func (p *channelsPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	type result struct {
		rows      []IMConnection
		adoptable []struct{ Channel, Identity, Service string }
		wechat    WechatState
		settings  WechatSettings
	}
	core.Go(p.settings.parent.env, func() (result, error) {
		var r result
		var list struct {
			Connections []IMConnection `json:"connections"`
		}
		if err := p.settings.parent.call("imConnection.list", nil, &list); err != nil {
			return r, err
		}
		r.rows = list.Connections
		var available struct {
			Bots []struct{ Channel, Identity, Service string } `json:"bots"`
		}
		if err := p.settings.parent.call("imConnection.listAdoptable", nil, &available); err != nil {
			return r, err
		}
		r.adoptable = available.Bots
		if err := p.settings.parent.call("wechat.state", nil, &r.wechat); err != nil {
			return r, err
		}
		if err := p.settings.parent.call("wechat.settings", nil, &r.settings); err != nil {
			return r, err
		}
		return r, nil
	}, func(r result, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.rows, p.adoptable, p.wechat, p.wechatSettings = r.rows, r.adoptable, r.wechat, r.settings
		p.error = ""
	})
}
func (p *channelsPane) action(method string, params any, done func()) {
	if p.closed || p.busy {
		return
	}
	p.busy = true
	oneBotCall(p.settings.parent, method, params, func(_ json.RawMessage, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.error = ""
		if done != nil {
			done()
		}
		p.Refresh()
	})
}
func (p *channelsPane) loadDetails(id string) {
	// Until this connection's replies arrive, no previous connection's lane
	// can supply a reset/archive action under the new connection identity.
	p.selected = id
	p.lanes = nil
	p.groups = nil
	oneBotCall(p.settings.parent, "imConnection.listLanes", map[string]string{"id": id}, func(value struct {
		Lanes []IMLane `json:"lanes"`
	}, err error) {
		if p.closed || p.selected != id {
			return
		}
		if err != nil {
			p.error = err.Error()
		} else {
			p.lanes = value.Lanes
		}
	})
	oneBotCall(p.settings.parent, "imConnection.listGroups", map[string]string{"id": id}, func(value struct {
		Groups []IMGroup `json:"groups"`
	}, err error) {
		if p.closed || p.selected != id {
			return
		}
		if err != nil {
			p.error = err.Error()
		} else {
			p.groups = value.Groups
		}
	})
}
func platformName(channel, service string) string {
	if channel == "feishu" && service == "lark" {
		return bt("bots.channels.platform.lark")
	}
	return bt("bots.channels.platform." + channel)
}
func maskContext(id string) string {
	chars := []rune(id)
	if len(chars) <= 8 {
		return id
	}
	return string(chars[:3]) + "…" + string(chars[len(chars)-3:])
}
func (p *channelsPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale {
		p.Refresh()
	}
	kit.Help(c, bt("bots.channels.empty", "name", p.settings.draft.Name))
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, nt("addConnection"), icons.Plus, false).Clicked() {
			p.adding = true
		}
		if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
			p.Refresh()
		}
	})
	kit.InlineError(c, p.error)
	for _, row := range p.rows {
		if row.BotID != p.settings.botID {
			continue
		}
		ui.Column(c.Key(row.ID)).FillWidth().Gap(5).Padding(10, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			identity := maskContext(row.BotContextID)
			if row.Identity.Username != "" {
				identity = row.Identity.Username
			}
			kit.Strong(c, platformName(row.Channel, row.Service)+" · "+identity)
			kit.Caption(c, bt("bots.channels.status."+row.Status.Kind))
			kit.InlineError(c, row.Status.Reason)
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, nt("connectionTasks"), icons.ListTodo, false).Clicked() {
					p.loadDetails(row.ID)
				}
				if kit.Button(c, nt("reconnect"), icons.RefreshCw, false).Disabled(p.busy).Clicked() {
					p.action("im.reconnect", map[string]string{"id": row.ID}, nil)
				}
				if row.Channel == "telegram" {
					mode := "offline"
					if row.Status.Kind == "offline" {
						mode = "online"
					}
					if kit.Button(c, nt(mode), icons.Power, false).Disabled(p.busy).Clicked() {
						p.action("im."+mode, map[string]string{"id": row.ID}, nil)
					}
				}
				if kit.Button(c, nt("removeConnection"), icons.Unplug, false).Clicked() {
					capture := row
					p.remove = &capture
				}
			})
			if p.selected == row.ID {
				p.detailsView(c, row)
			}
		})
	}
	for _, available := range p.adoptable {
		if kit.Button(c, nt("adoptConnection", "platform", platformName(available.Channel, available.Service), "identity", available.Identity), icons.ArrowRight, false).Clicked() {
			p.adoptChannel = available.Channel
		}
	}
	p.wechatView(c)
	p.addView(c)
	p.removeView(c)
	if p.adoptChannel != "" {
		open := true
		channel := p.adoptChannel
		if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("adoptTitle"), Message: nt("adoptHint"), Confirm: nt("adoptConfirm"), Cancel: nt("cancel"), Busy: p.busy, Error: p.error}) {
			p.action("imConnection.adopt", map[string]any{"input": map[string]string{"botId": p.settings.botID, "channel": channel}}, func() { p.adoptChannel = "" })
		}
		if !open {
			p.adoptChannel = ""
		}
	}
}
func (p *channelsPane) addView(c *ui.Context) {
	if !p.adding {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("addConnection"), Width: 640, Confirm: nt("connect"), Cancel: nt("cancel"), Busy: p.busy, Error: p.error, Body: func() {
		options := []kit.SelectOption{}
		for _, channel := range []string{"telegram", "feishu", "wecom", "dingtalk", "discord"} {
			options = append(options, kit.SelectOption{Value: channel, Label: platformName(channel, "")})
		}
		ui.Field(c, nt("platform"), func() { kit.Select(c, &p.channel, options) })
		secret := func(label string, value *string) {
			ui.Field(c, label, func() {
				field := kit.TextInput(c, value).FillWidth().Label(label)
				if !p.showSecret {
					field.Password()
				}
			})
		}
		plain := func(label string, value *string) {
			ui.Field(c, label, func() { kit.TextInput(c, value).FillWidth().Label(label) })
		}
		switch p.channel {
		case "telegram", "discord":
			secret("Bot Token", &p.token)
			plain(nt("ownerUserID"), &p.ownerID)
		case "feishu":
			kit.Select(c, &p.service, []kit.SelectOption{{Value: "feishu", Label: platformName("feishu", "feishu")}, {Value: "lark", Label: platformName("feishu", "lark")}})
			plain("App ID", &p.appID)
			secret("App Secret", &p.appSecret)
		case "wecom":
			plain("Bot ID", &p.platformBotID)
			secret("Secret", &p.appSecret)
		case "dingtalk":
			plain("AppKey", &p.appKey)
			secret("AppSecret", &p.appSecret)
		}
		kit.Toggle(c, &p.showSecret, nt("showSecret"), "")
		kit.Help(c, bt("bots.channels.form."+p.channel+".hint"))
		if kit.Button(c, nt("platformSetup"), icons.ExternalLink, false).Clicked() {
			urls := map[string]string{"telegram": "https://t.me/BotFather", "discord": "https://discord.com/developers/applications", "feishu": "https://open.feishu.cn/page/launcher?from=backend_oneclick", "wecom": "https://developer.work.weixin.qq.com/", "dingtalk": "https://open-dev.dingtalk.com/"}
			address := urls[p.channel]
			if p.channel == "feishu" && p.service == "lark" {
				address = "https://open.larksuite.com/page/launcher?from=backend_oneclick"
			}
			p.settings.parent.openExternal(address)
		}
	}}) {
		input := map[string]string{"botId": p.settings.botID, "channel": p.channel}
		switch p.channel {
		case "telegram", "discord":
			input["token"], input["ownerUserId"] = p.token, p.ownerID
		case "feishu":
			input["appId"], input["appSecret"], input["service"] = p.appID, p.appSecret, p.service
		case "wecom":
			input["wecomBotId"], input["secret"] = p.platformBotID, p.appSecret
		case "dingtalk":
			input["appKey"], input["appSecret"] = p.appKey, p.appSecret
		}
		p.action("imConnection.add", map[string]any{"input": input}, p.Discard)
	}
	if !open && !p.busy {
		p.settings.parent.env.RequestChange(p.Discard)
	}
}
func (p *channelsPane) removeView(c *ui.Context) {
	capture := p.remove
	if capture == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: bt("bots.channels.removeConfirm.title", "platform", platformName(capture.Channel, capture.Service)), Message: bt("bots.channels.removeConfirm.description", "name", p.settings.draft.Name), Confirm: nt("removeConnection"), Cancel: nt("cancel"), Danger: true, Busy: p.busy, Error: p.error}) {
		p.action("imConnection.remove", map[string]string{"id": capture.ID}, func() { p.remove = nil; p.selected = "" })
	}
	if !open {
		p.remove = nil
	}
}
func (p *channelsPane) detailsView(c *ui.Context, row IMConnection) {
	for _, group := range p.groups {
		mode := group.Activation
		ui.Row(c.Key(group.ChatID)).FillWidth().Gap(8).Wrap().Children(func() {
			kit.Help(c, group.ChatName+" · "+group.ChatID).Grow(1)
			if changed, _ := kit.Select(c, &mode, []kit.SelectOption{{Value: "mention", Label: nt("mentionOnly")}, {Value: "always", Label: nt("allMessages")}}); changed {
				p.action("imConnection.setGroupActivation", map[string]string{"id": row.ID, "chatId": group.ChatID, "mode": mode}, func() { p.loadDetails(row.ID) })
			}
		})
	}
	for _, lane := range p.lanes {
		ui.Column(c.Key(lane.SessionID)).FillWidth().Gap(4).Children(func() {
			name := lane.Name
			if name == "" {
				name = lane.ChatID
			}
			if name == "" {
				name = nt("ownerDM")
			}
			if kit.GhostButton(c, name, icons.MessageCircle, false).FillWidth().Justify(ui.Start).Clicked() {
				p.settings.parent.openSession(p.settings.botID, lane.SessionID)
			}
			kit.Caption(c, nt("lane."+lane.Kind)+" · "+nt("laneState."+lane.State))
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				if kit.Button(c, nt("resetLane"), icons.RotateCcw, false).Disabled(p.busy).Clicked() {
					p.action("imConnection.resetLane", map[string]string{"id": row.ID, "sessionId": lane.SessionID}, func() { p.loadDetails(row.ID) })
				}
				if kit.Button(c, nt("archiveLane"), icons.Archive, false).Disabled(p.busy).Clicked() {
					p.action("imConnection.archiveLane", map[string]string{"id": row.ID, "sessionId": lane.SessionID}, func() { p.loadDetails(row.ID) })
				}
			})
		})
	}
}
func (p *channelsPane) wechatView(c *ui.Context) {
	kit.Heading(c, bt("settings.wechatBot.title"))
	phase := p.wechat.Phase
	switch phase {
	case "waiting_confirmation":
		phase = "waitingConfirmation"
	case "disabled_by_policy":
		phase = "disabled"
	}
	kit.Caption(c, bt("settings.wechatBot.status."+phase))
	if p.wechat.ErrorCode != "" {
		kit.InlineError(c, p.wechat.ErrorCode)
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.Button(c, bt("settings.wechatBot.actions.connect"), icons.Link, false).Disabled(p.busy).Clicked() {
			p.confirmWechat = "authorize"
		}
		if p.wechat.Phase == "waiting_confirmation" && kit.Button(c, bt("settings.wechatBot.waiting.cancel"), icons.X, false).Clicked() {
			p.action("wechat.cancelAuthorization", nil, nil)
		}
		if p.wechat.Bound && kit.Button(c, bt("settings.wechatBot.actions.unbind"), icons.Unlink, false).Clicked() {
			p.confirmWechat = "unbind"
		}
	})
	if p.wechat.Bound {
		bound := p.wechatSettings.CompanionBotID
		if bound == p.settings.botID {
			kit.Help(c, bt("settings.wechatBot.companion.description", "name", p.settings.draft.Name))
			if kit.Button(c, bt("settings.wechatBot.companion.remove"), icons.X, false).Clicked() {
				p.action("wechat.setCompanion", map[string]any{"botId": nil}, nil)
			}
		} else if bound == "" {
			if kit.Button(c, bt("settings.wechatBot.companion.useExisting", "name", p.settings.draft.Name), icons.Link, false).Clicked() {
				p.action("wechat.setCompanion", map[string]string{"botId": p.settings.botID}, nil)
			}
		} else {
			kit.Status(c, bt("settings.wechatBot.companion.other"))
		}
	}
	path := ""
	if p.wechatSettings.WorkingDir != nil {
		path = *p.wechatSettings.WorkingDir
	}
	field := ui.Box(c).FillWidth().Label(bt("settings.wechatBot.workingDir.title")).Children(func() { ui.Text(c, path).FillWidth().Selectable() })
	widgets.DropArea(c, p.settings.parent.env, field, widgets.DropFolders, false, p.busy, func(paths []string, err error) {
		if err != nil {
			p.error = err.Error()
			return
		}
		if len(paths) > 0 {
			p.action("wechat.setWorkingDirectory", map[string]string{"path": paths[0]}, nil)
		}
	})
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.Button(c, nt("chooseFolder"), icons.FolderOpen, false).Clicked() {
			core.Go(p.settings.parent.env, func() (string, error) {
				return p.settings.parent.env.Dialogs.PickFolder(bt("settings.wechatBot.workingDir.title"))
			}, func(path string, err error) {
				if p.closed {
					return
				}
				if err != nil {
					p.error = err.Error()
				} else if path != "" {
					p.action("wechat.setWorkingDirectory", map[string]string{"path": path}, nil)
				}
			})
		}
		if kit.Button(c, bt("settings.wechatBot.workingDir.reset"), icons.RotateCcw, false).Clicked() {
			p.action("wechat.resetWorkingDirectory", nil, nil)
		}
	})
	if !p.wechatSettings.WorkingDirAvailable {
		kit.Notice(c, kit.Caution, "", bt("settings.wechatBot.workingDir.unavailable"), nil)
	}
	if p.confirmWechat != "" {
		open := true
		method := p.confirmWechat
		title, hint := bt("settings.wechatBot.authorization.title"), bt("settings.wechatBot.authorization.description")
		if method == "unbind" {
			title, hint = bt("settings.wechatBot.unbind.title"), bt("settings.wechatBot.unbind.description")
		} else if p.wechat.Bound {
			hint = bt("settings.wechatBot.authorization.rebindDescription")
		}
		if kit.Dialog(c, &open, kit.DialogOptions{Title: title, Message: hint, Confirm: nt("confirm"), Cancel: nt("cancel"), Danger: method == "unbind", Busy: p.busy, Error: p.error}) {
			p.action("wechat."+method, nil, func() { p.confirmWechat = "" })
		}
		if !open {
			p.confirmWechat = ""
		}
	}
}

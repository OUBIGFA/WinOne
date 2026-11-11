package flashchat

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/skip2/go-qrcode"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (p *completePage) openSettings() {
	p.panel, p.settingsTab = "settings", "preferences"
	p.formNickname = p.snapshot.Settings.Nickname
	p.trackers = strings.Join(p.snapshot.Settings.Trackers, "\n")
	p.rtc = string(p.snapshot.Settings.RTC)
	if p.rtc == "null" {
		p.rtc = ""
	}
}
func (p *completePage) openIdentity(peer nativePeer) {
	p.command("shell", "identity", map[string]any{"peer": peer.PeerID}, func(raw json.RawMessage) {
		_ = json.Unmarshal(raw, &p.keyText)
		p.identityName, p.panel = peer.Name, "identity"
	})
}
func (p *completePage) setting(key string, value any) {
	p.command("shell", "settings", map[string]any{"value": map[string]any{key: value}}, nil)
}

func (p *completePage) nativeDialogs(c *ui.Context) {
	if p.panel == "" {
		return
	}
	open := true
	panel := p.panel
	o := kit.DialogOptions{Title: nt(panel), Cancel: nt("done"), Width: 680, Error: p.actionError, Busy: p.busy}
	switch panel {
	case "preview":
		o.Title = nt("preview")
		o.Width = 1000
		o.Body = func() {
			if frame := p.frames[p.previewID]; frame != nil {
				ui.Image(c, frame).FillWidth().Height(500).MaxHeightPercent(100).Fit(ui.Contain)
			}
		}
	case "settings":
		o.Header = func() {
			kit.Segmented(c, &p.settingsTab, []kit.SelectOption{{Value: "preferences", Label: nt("preferences")}, {Value: "network", Label: nt("network")}, {Value: "profile", Label: nt("profile")}}, nt("settings"))
		}
		o.Body = func() { p.settingsBody(c) }
	case "invite":
		o.Message = nt("inviteHint")
		o.Body = func() {
			link := p.invitation()
			ui.Text(c, link).Selectable().FillWidth().MinWidth(0)
			if kit.Button(c, nt("copyInvite"), icons.Copy, false).Clicked() {
				c.Services().WriteClipboard(link)
				c.Toast(tr("copied"))
			}
			// QR generation encodes the same portable invitation the copy action uses.
			if p.qrLink != link {
				if data, err := qrcode.Encode(link, qrcode.Medium, 256); err == nil {
					p.qr, _ = ui.DecodeBitmap(data)
					p.qrLink = link
				}
			}
			if p.qr != nil {
				ui.Image(c, p.qr).Size(220, 220).AlignSelf(ui.Center)
			}
			if kit.GhostButton(c, nt("embed"), icons.Code, false).Clicked() {
				p.panel = "embed"
			}
		}
	case "community":
		o.Message = nt("communityHint")
		o.Body = func() {
			for _, room := range p.snapshot.Community {
				if kit.Button(c, room, icons.MessagesCircle, false).FillWidth().Disabled(p.busy || !p.snapshot.Ready).Clicked() {
					p.formRoom, p.roomKind, p.panel = room, "public", ""
				}
			}
		}
	case "identity":
		o.Message = p.identityName
		o.Body = func() {
			kit.Help(c, nt("identityHint"))
			ui.Text(c, p.keyText).Selectable().FillWidth().MinWidth(0)
			if kit.Button(c, nt("copyKey"), icons.Copy, false).Clicked() {
				c.Services().WriteClipboard(p.keyText)
			}
		}
	case "devices":
		o.Body = func() {
			for _, deviceKind := range []struct {
				label, action string
				devices       []nativeDevice
			}{{"microphone", "audioDevice", p.snapshot.Media.AudioDevices}, {"camera", "videoDevice", p.snapshot.Media.VideoDevices}} {
				kit.Label(c, nt(deviceKind.label))
				if len(deviceKind.devices) == 0 {
					kit.Help(c, nt("enableDeviceHint"))
				}
				for _, device := range deviceKind.devices {
					label := device.Label
					if label == "" {
						label = nt("defaultDevice")
					}
					if kit.Button(c, label, nil, false).FillWidth().Disabled(p.busy).Clicked() {
						p.command("media", deviceKind.action, map[string]any{"id": device.ID}, nil)
					}
				}
			}
			kit.Label(c, nt("volume"))
			for _, peer := range p.snapshot.Peers {
				for _, channel := range []struct{ id, label string }{{"microphone", "microphone"}, {"screen-share", "screen"}} {
					if peer.AudioChannels[channel.id] != "PLAYING" {
						continue
					}
					key := peer.PeerID + ":" + channel.id
					value, exists := p.volumes[key]
					if !exists {
						value = 1
					}
					label := peer.Name + " · " + nt(channel.label)
					kit.Help(c, label)
					if ui.Slider(c.Key(key), &value, 0, 1).FillWidth().Label(nt("volume") + " · " + label).Changed() {
						p.volumes[key] = value
						p.backgroundCommand("shell", "volume", map[string]any{"peer": peer.PeerID, "channel": channel.id, "value": value})
					}
				}
			}
		}
	case "embed":
		o.Message = nt("embedHint")
		o.Body = func() {
			nativeField(c, nt("webAddress"), &p.embedRoot, false)
			root, err := url.Parse(p.embedRoot)
			valid := err == nil && root.Scheme == "https" && root.Host != "" && root.User == nil
			room := p.snapshot.RoomID
			if room == "" {
				room = p.formRoom
			}
			if valid {
				base := strings.TrimRight(root.String(), "/") + "/"
				path := "public/" + url.PathEscape(room) + "?embed=1"
				if p.snapshot.Secret != "" {
					path = "private/" + url.PathEscape(room) + "?embed=1#secret=" + url.QueryEscape(p.snapshot.Secret)
				}
				code := fmt.Sprintf("<iframe src=\"%s\" allow=\"camera; microphone; display-capture; fullscreen\" width=\"800\" height=\"800\"></iframe>", html.EscapeString(base+path))
				// The upstream SDK only joins public rooms; never silently turn a
				// private invitation into public-room code.
				if p.snapshot.Secret == "" {
					code += fmt.Sprintf("\n\n<script src=\"%ssdk/sdk.js\"></script>\n<chat-room room=\"%s\" root-url=\"%s\" width=\"800\" height=\"800\"></chat-room>", html.EscapeString(base), html.EscapeString(room), html.EscapeString(base))
				}
				ui.Text(c, code).Selectable().FillWidth().MinWidth(0)
				if kit.Button(c, nt("copyCode"), icons.Copy, false).Clicked() {
					c.Services().WriteClipboard(code)
				}
			}
		}
	case "reset":
		o.Message, o.Confirm, o.Danger = nt("resetHint"), nt("reset"), true
		o.Cancel = i18n.T("common.cancel")
	case "help":
		o.Body = func() {
			for _, key := range []string{"helpRooms", "helpPrivacy", "helpMedia", "helpFiles", "helpNetwork"} {
				kit.Help(c, nt(key))
			}
			if kit.Button(c, nt("embed"), icons.Code, false).Clicked() {
				p.panel = "embed"
			}
		}
	}
	if kit.Dialog(c, &open, o) && panel == "reset" {
		p.command("shell", "reset", nil, func(json.RawMessage) { p.panel = ""; p.snapshot = nativeSnapshot{} })
	}
	if !open {
		p.panel = ""
	}
}

func (p *completePage) settingsBody(c *ui.Context) {
	switch p.settingsTab {
	case "preferences":
		ui.Column(c).FillWidth().Gap(24).Children(func() {
			kit.Group(c, nil, nt("identitySettings"), "", func() {
				kit.Stack(c, tr("nickname"), "", func() {
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						kit.TextInput(c, &p.formNickname).FillWidth().Label(tr("nickname"))
						if kit.Button(c, nt("applyNickname"), nil, false).Disabled(p.busy || p.formNickname == p.snapshot.Settings.Nickname).Clicked() {
							p.setting("customUsername", p.formNickname)
						}
					})
				})
			})
			kit.Group(c, nil, nt("notifications"), "", func() {
				for _, setting := range []struct {
					key, label string
					value      bool
				}{{"playSoundOnNewMessage", "sound", p.snapshot.Settings.Sound}, {"showNotificationOnNewMessage", "notification", p.snapshot.Settings.Notification}, {"showActiveTypingStatus", "typingSetting", p.snapshot.Settings.Typing}} {
					kit.Row(c, nt(setting.label), "", func() {
						value := setting.value
						if ui.Switch(c, &value).Label(nt(setting.label)).Disabled(p.busy || !p.snapshot.Ready).Changed() {
							p.setting(setting.key, value)
						}
					})
				}
				kit.Stack(c, nt("soundChoice"), "", func() {
					options := []kit.SelectOption{}
					for i, sound := range p.snapshot.Sounds {
						options = append(options, kit.SelectOption{Value: sound.Value, Label: nt(fmt.Sprintf("sound%d", i))})
					}
					selected := p.snapshot.Settings.SelectedSound
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						if changed, _ := kit.Select(c, &selected, options); changed {
							p.setting("selectedSound", selected)
						}
						if kit.IconButton(c, icons.Volume2, nt("previewSound")).Disabled(p.busy).Clicked() {
							p.command("shell", "sound", map[string]any{"value": selected}, nil)
						}
					})
				})
			})
		})
	case "network":
		kit.Help(c, nt("networkHint"))
		if p.snapshot.EnhancedAvailable {
			value := p.snapshot.Settings.Enhanced
			if ui.Checkbox(c, &value, nt("enhanced")).Disabled(p.busy).Changed() {
				p.setting("isEnhancedConnectivityEnabled", value)
			}
		}
		kit.Label(c, nt("trackers"))
		ui.TextArea(c, &p.trackers).FillWidth().Lines(2, 5).Label(nt("trackers"))
		kit.Label(c, nt("rtc"))
		ui.TextArea(c, &p.rtc).FillWidth().Lines(3, 8).Label(nt("rtc"))
		if kit.PrimaryButton(c, nt("applyNetwork"), nil, p.busy).Disabled(p.busy || !p.snapshot.Ready).Clicked() {
			trackers := []string{}
			for _, line := range strings.Split(p.trackers, "\n") {
				if line = strings.TrimSpace(line); line != "" {
					trackers = append(trackers, line)
				}
			}
			rtc := json.RawMessage("{}")
			if strings.TrimSpace(p.rtc) != "" {
				rtc = json.RawMessage(p.rtc)
			}
			if !json.Valid(rtc) {
				p.actionError = nt("invalidJSON")
			} else {
				p.command("shell", "settings", map[string]any{"value": map[string]any{"trackerUrls": trackers, "rtcConfig": rtc}}, nil)
			}
		}
		kit.Help(c, nt("connectionStatus", "status", p.snapshot.Connections.TrackerConnection))
	case "profile":
		kit.Help(c, nt("profileHint"))
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, nt("export"), icons.Download, false).Disabled(p.busy || p.env.Dialogs == nil).Clicked() {
				p.profileFile(false)
			}
			if kit.Button(c, nt("import"), icons.Upload, false).Disabled(p.busy || p.env.Dialogs == nil).Clicked() {
				p.profileFile(true)
			}
		})
		if kit.Button(c, nt("myIdentity"), icons.ShieldCheck, false).Disabled(p.busy).Clicked() {
			p.openIdentity(nativePeer{Name: p.snapshot.Settings.Nickname})
		}
		if kit.DangerButton(c, nt("reset"), nil, false).Disabled(p.busy).Clicked() {
			p.panel = "reset"
		}
	}
}

// The native file picker grants exactly one profile file; no directory scan or
// arbitrary path can be requested by the browser engine.
func (p *completePage) profileFile(importing bool) {
	if p.busy {
		return
	}
	p.busy, p.actionError = true, ""
	core.Do(p.env, func() error {
		filter := core.Filter{Name: "JSON", Extensions: []string{"json"}}
		if importing {
			path, err := p.env.Dialogs.PickFile(nt("import"), filter)
			if err != nil || path == "" {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				return err
			}
			_, err = p.invoke("shell", "import", map[string]any{"value": value})
			return err
		}
		path, err := p.env.Dialogs.SaveFile(nt("export"), filter, "flashchat-profile.json")
		if err != nil || path == "" {
			return err
		}
		data, err := p.invoke("shell", "export", nil)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0600)
	}, func(err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.actionError = err.Error()
		} else {
			p.formNickname = p.snapshot.Settings.Nickname
		}
	})
}

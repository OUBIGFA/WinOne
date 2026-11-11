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
			// Second-level tabs, as the settings pages switch their panels.
			keys := []string{"preferences", "network", "profile"}
			index := 0
			for i, key := range keys {
				if key == p.settingsTab {
					index = i
				}
			}
			if kit.SubTabs(c, &index, []string{nt("preferences"), nt("network"), nt("profile")}, nil) {
				p.settingsTab = keys[index]
			}
		}
		o.Body = func() { p.settingsBody(c) }
	case "invite":
		o.Message = nt("inviteHint")
		o.Body = func() {
			link := p.invitation()
			codeBox(c, link)
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, nt("copyInvite"), icons.Copy, false).Clicked() {
					c.Services().WriteClipboard(link)
					c.Toast(tr("copied"))
				}
				if kit.GhostButton(c, nt("embed"), icons.Code, false).Clicked() {
					p.panel = "embed"
				}
			})
			// QR generation encodes the same portable invitation the copy action uses.
			if p.qrLink != link {
				if data, err := qrcode.Encode(link, qrcode.Medium, 256); err == nil {
					p.qr, _ = ui.DecodeBitmap(data)
					p.qrLink = link
				}
			}
			if p.qr != nil {
				// A QR code is scanned on white in both themes.
				ui.Box(c).AlignSelf(ui.Center).Margin(8, 0, 0, 0).Padding(8).Radius(kit.PanelRadius).Background(ui.RGB(255, 255, 255)).
					Border(1, kit.Colors(c).Hairline).Children(func() { ui.Image(c, p.qr).Size(200, 200) })
			}
		}
	case "identity":
		o.Message = p.identityName
		o.Body = func() {
			kit.Help(c, nt("identityHint"))
			codeBox(c, p.keyText)
			if kit.Button(c, nt("copyKey"), icons.Copy, false).Clicked() {
				c.Services().WriteClipboard(p.keyText)
			}
		}
	case "devices":
		o.Body = func() {
			kit.Panel(c, func() {
				for _, deviceKind := range []struct {
					label, action string
					devices       []nativeDevice
				}{{"microphone", "audioDevice", p.snapshot.Media.AudioDevices}, {"camera", "videoDevice", p.snapshot.Media.VideoDevices}} {
					help := ""
					if len(deviceKind.devices) == 0 {
						help = nt("enableDeviceHint")
					}
					kit.Stack(c, nt(deviceKind.label), help, func() {
						for _, device := range deviceKind.devices {
							label := device.Label
							if label == "" {
								label = nt("defaultDevice")
							}
							if kit.Button(c, label, nil, false).FillWidth().Disabled(p.busy).Clicked() {
								p.command("media", deviceKind.action, map[string]any{"id": device.ID}, nil)
							}
						}
					})
				}
				kit.Stack(c, nt("volume"), "", func() {
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
				})
			})
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
				codeBox(c, code)
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
				kit.Body(c, nt(key)).Selectable()
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
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						kit.TextInput(c, &p.formNickname).Grow(1).MinWidth(0).Label(tr("nickname"))
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
					value := setting.value
					if kit.Toggle(c.Key(setting.key), &value, nt(setting.label), "").Disabled(p.busy || !p.snapshot.Ready).Changed() {
						p.setting(setting.key, value)
					}
				}
				kit.Row(c, nt("soundChoice"), "", func() {
					options := []kit.SelectOption{}
					for i, sound := range p.snapshot.Sounds {
						options = append(options, kit.SelectOption{Value: sound.Value, Label: nt(fmt.Sprintf("sound%d", i))})
					}
					selected := p.snapshot.Settings.SelectedSound
					changed, trigger := kit.Select(c, &selected, options)
					trigger.Label(nt("soundChoice")).Disabled(p.busy || len(options) == 0)
					if changed {
						p.setting("selectedSound", selected)
					}
					if kit.IconButton(c, icons.Volume2, nt("previewSound")).Disabled(p.busy).Clicked() {
						p.command("shell", "sound", map[string]any{"value": selected}, nil)
					}
				})
			})
		})
	case "network":
		ui.Column(c).FillWidth().Gap(kit.HeadingGap).Children(func() {
			kit.Help(c, nt("networkHint"))
			kit.Panel(c, func() {
				if p.snapshot.EnhancedAvailable {
					value := p.snapshot.Settings.Enhanced
					if kit.Toggle(c, &value, nt("enhanced"), "").Disabled(p.busy).Changed() {
						p.setting("isEnhancedConnectivityEnabled", value)
					}
				}
				kit.Stack(c, nt("trackers"), "", func() {
					ui.TextArea(c, &p.trackers).FillWidth().Lines(2, 5).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Label(nt("trackers"))
				})
				kit.Stack(c, nt("rtc"), "", func() {
					ui.TextArea(c, &p.rtc).FillWidth().Lines(3, 8).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Label(nt("rtc"))
				})
				kit.Row(c, nt("connectionStatus", "status", p.snapshot.Connections.TrackerConnection), "", func() {
					if kit.PrimaryButton(c, nt("applyNetwork"), nil, p.busy).Disabled(p.busy || !p.snapshot.Ready).Clicked() {
						p.applyNetwork()
					}
				})
			})
		})
	case "profile":
		kit.Panel(c, func() {
			kit.Stack(c, nt("transferProfile"), nt("profileHint"), func() {
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					if kit.Button(c, nt("export"), icons.Download, false).Disabled(p.busy || p.env.Dialogs == nil).Clicked() {
						p.profileFile(false)
					}
					if kit.Button(c, nt("import"), icons.Upload, false).Disabled(p.busy || p.env.Dialogs == nil).Clicked() {
						p.profileFile(true)
					}
				})
			})
			kit.Row(c, nt("identity"), nt("identityHint"), func() {
				if kit.Button(c, nt("myIdentity"), icons.ShieldCheck, false).Disabled(p.busy).Clicked() {
					p.openIdentity(nativePeer{Name: p.snapshot.Settings.Nickname})
				}
			})
			kit.Row(c, nt("reset"), "", func() {
				if kit.DangerButton(c, nt("reset"), nil, false).Disabled(p.busy).Clicked() {
					p.panel = "reset"
				}
			})
		})
	}
}

// applyNetwork saves the pairing servers and WebRTC configuration as typed.
func (p *completePage) applyNetwork() {
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
		return
	}
	p.command("shell", "settings", map[string]any{"value": map[string]any{"trackerUrls": trackers, "rtcConfig": rtc}}, nil)
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

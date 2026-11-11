package flashchat

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Layout reference: Slack Design's conversation/list split, quiet toolbar and
// contiguous composer (2023-10). This is composition of MyGo controls, not a
// copied stylesheet or web surface; colours and type remain WinOne's shared kit.
func (p *completePage) nativeLobby(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		ui.Row(c).FillWidth().Padding(24, 28).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.MessagesCircle).FontSize(22)
			kit.Heading(c, tr("peerChat")).Grow(1)
			if kit.IconButton(c, icons.Settings2, nt("settings")).Clicked() {
				p.openSettings()
			}
			if kit.IconButton(c, icons.Info, nt("help")).Clicked() {
				p.panel = "help"
			}
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Row(c).FillWidth().MaxWidth(880).Margin(36, ui.Auto, 24, ui.Auto).Padding(0, 28).Gap(56).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Basis(420).MinWidth(0).Gap(28).Children(func() {
					ui.Column(c).Gap(8).Children(func() { kit.Title(c, nt("startTitle")); kit.Help(c, nt("startHint")) })
					p.feedback(c)
					ui.Column(c).FillWidth().Gap(20).Children(func() {
						nativeField(c, tr("nickname"), &p.formNickname, false)
						ui.Column(c).FillWidth().Gap(8).Children(func() {
							kit.Label(c, nt("room"))
							ui.Row(c).FillWidth().Gap(6).Children(func() {
								kit.TextInput(c, &p.formRoom).Label(nt("room")).Placeholder(nt("roomPlaceholder")).FillWidth().MinWidth(0).Height(42)
								if kit.IconButton(c, icons.RefreshCw, nt("regenerate")).Size(42, 42).Disabled(!p.snapshot.Ready || p.busy).Clicked() {
									p.generateNative()
								}
							})
							ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
								kit.Caption(c, nt("nameType"))
								if kit.Segmented(c, &p.nameKind, []kit.SelectOption{{Value: "id", Label: nt("randomID")}, {Value: "words", Label: nt("randomWords")}}, nt("nameType")) {
									p.generateNative()
								}
							})
						})
						ui.Column(c).FillWidth().Gap(10).Children(func() {
							kit.Segmented(c, &p.roomKind, []kit.SelectOption{{Value: "public", Label: nt("public")}, {Value: "private", Label: nt("private")}}, nt("roomType"))
							if p.roomKind == "private" {
								nativeField(c, tr("roomPassword"), &p.formPassword, true)
							}
						})
						if kit.PrimaryButton(c, nt("enter"), icons.ArrowRight, p.busy).FillWidth().Height(42).Justify(ui.Center).Disabled(!p.snapshot.Ready || p.busy || strings.TrimSpace(p.formRoom) == "").Clicked() {
							p.command("shell", "join", map[string]any{"room": p.formRoom, "private": p.roomKind == "private", "password": p.formPassword, "nickname": p.formNickname}, nil)
						}
						if !p.snapshot.Ready && p.err == "" {
							kit.Caption(c, nt("starting"))
						}
					})
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icons.ShieldCheck).FontSize(15).TextColor(c.Theme().TextMuted)
						kit.Caption(c, tr("chatEncryption")).Shrink(1)
					})
					if p.area.Bounds(c).W < 800 {
						if kit.GhostButton(c, nt("community"), icons.Users, false).Clicked() {
							p.panel = "community"
						}
					}
				})
				if p.area.Bounds(c).W >= 800 {
					ui.Column(c).Width(240).Shrink(0).Gap(18).Padding(4, 0).Children(func() {
						kit.Heading(c, nt("community"))
						kit.Help(c, nt("communityHint"))
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							for _, room := range p.snapshot.Community {
								b := ui.ButtonBase(c).FillWidth().Padding(12, 10).Radius(8).Label(room).Gap(10).AlignItems(ui.Center)
								if b.Hovered() {
									b.Background(c.Theme().SurfaceHover)
								}
								b.Children(func() {
									ui.Icon(c, icons.MessagesCircle).FontSize(16).TextColor(c.Theme().TextMuted)
									kit.Body(c, room).Grow(1).MinWidth(0)
									ui.Icon(c, icons.ArrowRight).FontSize(14).TextColor(c.Theme().TextMuted)
								})
								if b.Clicked() {
									p.formRoom, p.roomKind = room, "public"
								}
							}
						})
					})
				}
			})
		})
	})
}

func (p *completePage) generateNative() {
	p.command("shell", "generate", map[string]any{"words": p.nameKind == "words"}, func(raw json.RawMessage) { _ = json.Unmarshal(raw, &p.formRoom) })
}

func (p *completePage) nativeWorkspace(c *ui.Context) {
	narrow := p.area.Bounds(c).W > 0 && p.area.Bounds(c).W < 760
	ui.Row(c).Fill().MinWidth(0).Children(func() {
		if !narrow || p.showPeers {
			p.nativeSidebar(c, narrow)
		}
		if !narrow || !p.showPeers {
			ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() { p.nativeHeader(c, narrow); p.nativeConversation(c) })
		}
	})
}

func (p *completePage) nativeHeader(c *ui.Context, narrow bool) {
	name, description := tr("groupRoom"), nt("members", "count", len(p.snapshot.Peers)+1)
	for _, peer := range p.snapshot.Peers {
		if peer.PeerID == p.selected {
			name = peer.Name
			description = tr("chatPrivate")
		}
	}
	ui.Row(c).FillWidth().MinHeight(76).Padding(14, 24).Gap(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
		if narrow && kit.IconButton(c, icons.Users, tr("chatPeers")).Clicked() {
			p.showPeers = true
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() { kit.Heading(c, name).SingleLine().Ellipsis("…").Tooltip(name); kit.Caption(c, description) })
		for _, control := range []struct {
			key    string
			active bool
			icon   *ui.SVG
		}{{"microphone", p.snapshot.Media.Microphone, icons.Mic}, {"camera", p.snapshot.Media.Camera, icons.Video}, {"screen", p.snapshot.Media.Screen, icons.Monitor}} {
			label := nt(control.key)
			if control.active {
				label = nt("stop") + " · " + label
			}
			b := kit.IconButton(c, control.icon, label).Disabled(p.busy)
			if control.active {
				b.Background(kit.Colors(c).Selected)
			}
			if b.Clicked() {
				p.command("media", control.key, map[string]any{"enabled": !control.active}, nil)
			}
		}
		kit.IconButton(c, icons.Ellipsis, nt("more")).PopoverMenuButton(func(m *ui.Menu) {
			for _, peer := range p.snapshot.Peers {
				if peer.PeerID == p.selected {
					if m.Item(nt("memberInfo")).Chosen() {
						p.openIdentity(peer)
					}
				}
			}
			if len(p.snapshot.Media.Streams) > 0 && m.Item(nt("toggleMessages")).Checked(p.hideMessages).Chosen() {
				p.hideMessages = !p.hideMessages
			}
			if m.Item(nt("invite")).Chosen() {
				p.panel = "invite"
			}
			if m.Item(nt("devices")).Chosen() {
				p.panel = "devices"
			}
			if m.Item(nt("fullscreen")).Chosen() {
				if w := mygo.FocusedWindow(); w != nil {
					w.ToggleFullScreen()
				}
			}
			if m.Item(nt("settings")).Chosen() {
				p.openSettings()
			}
			if m.Item(nt("help")).Chosen() {
				p.panel = "help"
			}
			m.Separator()
			if m.Item(tr("leaveRoom")).Danger().Disabled(p.busy).Chosen() {
				p.command("shell", "leave", nil, nil)
			}
		})
	})
}

func (p *completePage) selectNative(peer string) {
	p.command("shell", "select", map[string]any{"peer": peer}, func(json.RawMessage) { p.selected = peer; p.showPeers = false; p.previewRequested = map[string]bool{} })
}

func nativeAvatar(c *ui.Context, name string, size float32) {
	letter, _ := utf8.DecodeRuneInString(strings.TrimSpace(name))
	if letter == utf8.RuneError || letter == 0 {
		letter = '?'
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Background(kit.Colors(c).Selected).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Text(c, strings.ToUpper(string(letter))).FontWeight(500).FontSize(size * .4) })
}

func (p *completePage) nativeSidebar(c *ui.Context, narrow bool) {
	side := ui.Column(c).FillHeight().MinWidth(0).Background(kit.Colors(c).Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(kit.Colors(c).Hairline)
	if narrow {
		side.Grow(1)
	} else {
		side.Width(264).Shrink(0)
	}
	side.Children(func() {
		ui.Row(c).FillWidth().MinHeight(76).Padding(18, 20).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.MessagesCircle).FontSize(21)
			kit.Heading(c, tr("peerChat")).Grow(1)
			if kit.IconButton(c, icons.Share2, nt("invite")).Clicked() {
				p.panel = "invite"
			}
		})
		ui.Column(c).FillWidth().Padding(4, 20, 22, 20).Gap(6).Children(func() {
			kind := "public"
			if p.snapshot.Secret != "" {
				kind = "private"
			}
			kit.Caption(c, nt(kind))
			kit.Body(c, p.snapshot.RoomID).SingleLine().Ellipsis("…").Tooltip(p.snapshot.RoomID).Selectable()
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 10).Gap(4).Children(func() {
			p.conversationRow(c, "", tr("groupRoom"), icons.MessagesCircle)
			kit.Caption(c, nt("directMessages")).Padding(18, 12, 8, 12)
			for _, peer := range p.snapshot.Peers {
				p.conversationRow(c, peer.PeerID, peer.Name, nil)
			}
			if len(p.snapshot.Peers) == 0 {
				kit.Help(c, nt("waiting")).Padding(8, 12)
			}
		})
		ui.Row(c).FillWidth().Padding(16).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			name := p.snapshot.Settings.Nickname
			if name == "" {
				name = nt("you")
			}
			nativeAvatar(c, name, 32)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() { kit.Body(c, name).SingleLine().Ellipsis("…"); kit.Caption(c, nt("you")) })
			if kit.IconButton(c, icons.Settings2, nt("settings")).Clicked() {
				p.openSettings()
			}
		})
	})
}

func (p *completePage) conversationRow(c *ui.Context, id, name string, icon *ui.SVG) {
	room := p.snapshot.Rooms[id]
	preview := nt("newConversation")
	stamp := ""
	if id != "" {
		for _, peer := range p.snapshot.Peers {
			if peer.PeerID == id {
				preview = nt("verifying")
				if peer.Verified == 2 {
					preview = tr("verified")
				} else if peer.Verified == 1 {
					preview = nt("unverified")
				}
			}
		}
	}
	if len(room.Messages) > 0 {
		message := room.Messages[len(room.Messages)-1]
		preview = message.Text
		if message.Magnet != "" {
			preview = nt("sharedMedia")
		}
		stamp = time.UnixMilli(message.TimeSent).Format("15:04")
	}
	b := ui.ButtonBase(c.Key("conversation:"+id)).FillWidth().Padding(12, 10).Gap(10).Radius(9).AlignItems(ui.Center).Label(tr("chatPrivate") + " · " + name).Tooltip(name).Disabled(p.busy)
	if id == "" {
		b.Label(tr("groupRoom"))
	}
	if p.selected == id {
		b.Background(kit.Colors(c).Selected)
	} else if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() {
		if icon != nil {
			ui.Box(c).Size(36, 36).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Icon(c, icon).FontSize(21) })
		} else {
			nativeAvatar(c, name, 36)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				kit.Strong(c, name).Grow(1).Shrink(1).MinWidth(0).SingleLine().Ellipsis("…")
				if room.Unread > 0 {
					kit.Strong(c, nt("unreadCount", "count", room.Unread)).FontSize(12)
				} else if stamp != "" {
					kit.Caption(c, stamp).FontSize(12).Shrink(0)
				}
			})
			kit.Caption(c, strings.ReplaceAll(preview, "\n", " ")).FillWidth().SingleLine().Ellipsis("…")
		})
	})
	if b.Clicked() {
		p.selectNative(id)
	}
	b.PopoverMenu(func(m *ui.Menu) {
		for _, peer := range p.snapshot.Peers {
			if peer.PeerID == id {
				if m.Item(nt("memberInfo")).Chosen() {
					p.openIdentity(peer)
				}
				if peer.OfferedFileID != "" && m.Item(nt("download")).Chosen() {
					p.command("room:", "download", map[string]any{"magnet": peer.OfferedFileID}, nil)
				}
			}
		}
	})
	// A received file is a task, so expose its action next to this member rather
	// than requiring the context menu to discover it.
	for _, peer := range p.snapshot.Peers {
		if peer.PeerID == id && peer.OfferedFileID != "" {
			if kit.GhostButton(c, nt("download"), icons.Download, false).Margin(0, 10, 6, 56).Disabled(p.busy || p.downloading).Clicked() {
				p.command("room:", "download", map[string]any{"magnet": peer.OfferedFileID}, nil)
			}
		}
	}
}

func (p *completePage) nativeMessage(c *ui.Context, message nativeMessage) {
	name := message.AuthorID
	if message.AuthorID == p.snapshot.UserID {
		name = p.snapshot.Settings.Nickname
		if name == "" {
			name = nt("you")
		}
	} else {
		for _, peer := range p.snapshot.Peers {
			if peer.UserID == message.AuthorID {
				name = peer.Name
			}
		}
	}
	ui.Row(c).FillWidth().MinWidth(0).Gap(12).AlignItems(ui.Start).Children(func() {
		nativeAvatar(c, name, 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				kit.Strong(c, name).Shrink(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(name)
				kit.Caption(c, time.UnixMilli(message.TimeSent).Format("15:04")).Shrink(0)
			})
			if message.Magnet != "" {
				p.nativePreview(c, message)
				if kit.GhostButton(c, nt("downloadMedia"), icons.Download, false).Disabled(p.busy || p.downloading).Clicked() {
					p.command("room:"+p.selected, "download", map[string]any{"magnet": message.Magnet}, nil)
				}
			} else {
				markdown.View(c, message.Text, &p.md)
				if isVideoLink(message.Text) {
					if kit.Button(c, nt("openVideo"), icons.Play, false).Clicked() {
						c.Services().OpenURL(strings.TrimSpace(message.Text))
					}
				}
			}
		})
	})
}

func isVideoLink(text string) bool {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return (u.Host == "www.youtube.com" || u.Host == "youtube.com") && u.Path == "/watch" && u.Query().Get("v") != "" || u.Host == "youtu.be" && len(u.Path) > 1
}

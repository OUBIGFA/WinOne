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

// The page follows WinOne's own two surfaces instead of a look of its own:
// the lobby is a settings page (a centered column of kit groups on panels),
// and a room is laid out as the Chat page is (sidebar, quiet header, own
// messages in a bubble on the right, a white composer). Sizes, colors and
// controls all come from the shared kit.
const (
	// roomFormWidth bounds the lobby's column, as a form reads best narrow.
	roomFormWidth = 560
	// readingWidth bounds the transcript and composer, as Chat's column.
	readingWidth = 880
	// peersWidth is the room's list of conversations.
	peersWidth = 248
	// headerHeight matches the Chat page's header.
	headerHeight = 56
)

func (p *completePage) nativeLobby(c *ui.Context) {
	t := c.Theme()
	ready := p.snapshot.Ready
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		// Center the scroll view itself in the finite viewport: its height
		// shrinks to the viewport only when the content overflows, so the
		// first frame is already centered and nothing jumps afterwards.
		ui.Column(c).Fill().Justify(ui.Center).Children(func() {
			ui.Scroll(c).FillWidth().MaxHeightPercent(100).Shrink(1).Padding(32, 24).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(roomFormWidth).Margin(0, ui.Auto).Gap(kit.GroupGap).Children(func() {
					ui.Column(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, icons.MessagesCircle).FontSize(24).Shrink(0)
							kit.Title(c, tr("peerChat")).Role(ui.RoleHeading).Level(1)
						})
						kit.Help(c, nt("startHint")).TextAlign(ui.Center)
					})
					p.feedback(c)
					ui.Column(c).FillWidth().Gap(16).Children(func() {
						kit.Panel(c, func() {
							kit.Stack(c, tr("nickname"), "", func() {
								kit.TextInput(c, &p.formNickname).Label(tr("nickname")).FillWidth()
							})
							kit.Stack(c, nt("room"), "", func() {
								ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
									kit.TextInput(c, &p.formRoom).Label(nt("room")).Grow(1).MinWidth(0)
									if kit.IconButton(c, icons.RefreshCw, nt("regenerate")).Disabled(!ready || p.busy).Clicked() {
										p.generateNative()
									}
								})
							})
							kit.Row(c, nt("nameType"), "", func() {
								if kit.Segmented(c, &p.nameKind, []kit.SelectOption{{Value: "id", Label: nt("randomID")}, {Value: "words", Label: nt("randomWords")}}, nt("nameType")) {
									p.generateNative()
								}
							})
							kit.Row(c, nt("roomType"), "", func() {
								kit.Segmented(c, &p.roomKind, []kit.SelectOption{{Value: "public", Label: nt("public")}, {Value: "private", Label: nt("private")}}, nt("roomType"))
							})
							if p.roomKind == "private" {
								kit.Stack(c, tr("roomPassword"), "", func() {
									kit.TextInput(c, &p.formPassword).Label(tr("roomPassword")).FillWidth().Password()
								})
							}
						})
						ui.Column(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
							if kit.PrimaryButton(c, nt("enter"), icons.ArrowRight, p.busy).MinWidth(200).Justify(ui.Center).
								Disabled(!ready || p.busy || strings.TrimSpace(p.formRoom) == "").Clicked() {
								p.command("shell", "join", map[string]any{"room": p.formRoom, "private": p.roomKind == "private", "password": p.formPassword, "nickname": p.formNickname}, nil)
							}
							if !ready && p.err == "" {
								kit.Caption(c, nt("starting")).Role(ui.RoleStatus)
							}
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, icons.ShieldCheck).FontSize(15).TextColor(t.TextMuted).Shrink(0)
								kit.Caption(c, tr("chatEncryption")).Shrink(1)
							})
						})
					})
					if len(p.snapshot.Community) > 0 {
						kit.Group(c, nil, nt("community"), nt("communityHint"), func() {
							for _, room := range p.snapshot.Community {
								b := ui.ButtonBase(c.Key("community:"+room)).FillWidth().Padding(12, 16).Gap(12).AlignItems(ui.Center).
									Label(room).Disabled(p.busy)
								if p.formRoom == room && p.roomKind == "public" {
									b.Background(kit.Colors(c).Selected)
								} else if b.Hovered() {
									b.Background(t.SurfaceHover)
								}
								b.Children(func() {
									ui.Icon(c, icons.MessagesCircle).FontSize(16).TextColor(t.TextMuted).Shrink(0)
									kit.Label(c, room).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
									ui.Icon(c, icons.ChevronRight).FontSize(14).TextColor(t.TextMuted).Shrink(0)
								})
								if b.Clicked() {
									p.formRoom, p.roomKind = room, "public"
								}
							}
						})
					}
				})
			})
		})
		// Out of the flow, so the two tools do not push the form off center.
		ui.Row(c).Absolute().Top(12).Right(16).Gap(4).Children(func() {
			if kit.IconButton(c, icons.Settings2, nt("settings")).Clicked() {
				p.openSettings()
			}
			if kit.IconButton(c, icons.Info, nt("help")).Clicked() {
				p.panel = "help"
			}
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
	ui.Row(c).FillWidth().MinHeight(headerHeight).Shrink(0).Padding(8, 16, 8, 20).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
		if narrow && kit.IconButton(c, icons.Users, tr("chatPeers")).Clicked() {
			p.showPeers = true
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Padding(0, 8, 0, 0).Children(func() {
			kit.Label(c, name).SingleLine().Ellipsis("…").Tooltip(name).Role(ui.RoleHeading).Level(1)
			kit.Caption(c, description).SingleLine()
		})
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
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Background(kit.Colors(c).Selected).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Text(c, strings.ToUpper(string(letter))).FontWeight(500).FontSize(size * .42) })
}

func (p *completePage) nativeSidebar(c *ui.Context, narrow bool) {
	pal := kit.Colors(c)
	side := ui.Column(c).FillHeight().MinWidth(0).Background(pal.Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline)
	if narrow {
		side.Grow(1)
	} else {
		side.Width(peersWidth).Shrink(0)
	}
	side.Children(func() {
		// The room this window is in, where the Chat page names itself.
		ui.Row(c).FillWidth().MinHeight(headerHeight).Shrink(0).Padding(8, 8, 8, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			kind := "public"
			if p.snapshot.Secret != "" {
				kind = "private"
			}
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				kit.Label(c, p.snapshot.RoomID).SingleLine().Ellipsis("…").Tooltip(p.snapshot.RoomID).Selectable()
				kit.Caption(c, nt(kind)).SingleLine()
			})
			if kit.IconButton(c, icons.Share2, nt("invite")).Clicked() {
				p.panel = "invite"
			}
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Padding(4, 8).Gap(2).Children(func() {
			p.conversationRow(c, "", tr("groupRoom"), icons.MessagesCircle)
			kit.Caption(c, nt("directMessages")).Padding(16, 12, 6, 12)
			for _, peer := range p.snapshot.Peers {
				p.conversationRow(c, peer.PeerID, peer.Name, nil)
			}
			if len(p.snapshot.Peers) == 0 {
				kit.Help(c, nt("waiting")).Padding(4, 12)
			}
		})
		ui.Row(c).FillWidth().Shrink(0).Padding(8, 8, 8, 12).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
			name := p.snapshot.Settings.Nickname
			if name == "" {
				name = nt("you")
			}
			nativeAvatar(c, name, 28)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, name).SingleLine().Ellipsis("…").Tooltip(name)
				kit.Caption(c, nt("you"))
			})
			if kit.IconButton(c, icons.Settings2, nt("settings")).Clicked() {
				p.openSettings()
			}
		})
	})
}

func (p *completePage) conversationRow(c *ui.Context, id, name string, icon *ui.SVG) {
	t, pal := c.Theme(), kit.Colors(c)
	room := p.snapshot.Rooms[id]
	preview := nt("newConversation")
	stamp := ""
	offered := ""
	if id != "" {
		for _, peer := range p.snapshot.Peers {
			if peer.PeerID == id {
				offered = peer.OfferedFileID
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
	b := ui.ButtonBase(c.Key("conversation:"+id)).FillWidth().Padding(8, 12).Gap(10).Radius(t.Radius).AlignItems(ui.Center).Label(tr("chatPrivate") + " · " + name).Tooltip(name).Disabled(p.busy)
	if id == "" {
		b.Label(tr("groupRoom"))
	}
	if p.selected == id {
		b.Background(pal.Selected)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		if icon != nil {
			ui.Box(c).Size(28, 28).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Icon(c, icon).FontSize(18) })
		} else {
			nativeAvatar(c, name, 28)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Grow(1).Shrink(1).MinWidth(0).SingleLine().Ellipsis("…")
				if room.Unread > 0 {
					kit.Badge(c, nt("unreadCount", "count", room.Unread), kit.Info).Shrink(0)
				} else if stamp != "" {
					kit.Caption(c, stamp).Shrink(0)
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
	// A received file is a task, so its action shows under this member
	// instead of only in the context menu.
	if offered != "" {
		if kit.GhostButton(c, nt("download"), icons.Download, false).Margin(0, 8, 4, 42).Disabled(p.busy || p.downloading).Clicked() {
			p.command("room:", "download", map[string]any{"magnet": offered}, nil)
		}
	}
}

// bubbleColor is the fill of this side's own messages, as the Chat page's.
func bubbleColor(c *ui.Context) ui.Color {
	pal, t := kit.Colors(c), c.Theme()
	if t.Dark {
		return pal.Selected.Mix(t.Text, 0.06)
	}
	return pal.Selected.Mix(t.Background, 0.55)
}

// nativeMessage shows one message: this side's in a bubble on the right,
// a member's on the left under their name, as Chat tells the two apart.
func (p *completePage) nativeMessage(c *ui.Context, message nativeMessage) {
	mine := message.AuthorID == p.snapshot.UserID
	name := message.AuthorID
	if mine {
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
	stamp := time.UnixMilli(message.TimeSent).Format("15:04")
	body := func() {
		if message.Magnet != "" {
			p.nativePreview(c, message)
			if kit.GhostButton(c, nt("downloadMedia"), icons.Download, false).Disabled(p.busy || p.downloading).Clicked() {
				p.command("room:"+p.selected, "download", map[string]any{"magnet": message.Magnet}, nil)
			}
			return
		}
		markdown.View(c, message.Text, &p.md)
		if isVideoLink(message.Text) {
			if kit.Button(c, nt("openVideo"), icons.Play, false).Clicked() {
				c.Services().OpenURL(strings.TrimSpace(message.Text))
			}
		}
	}
	ui.Column(c).FillWidth().MinWidth(0).MaxWidth(readingWidth).Margin(0, ui.Auto).Children(func() {
		if mine {
			ui.Column(c).AlignSelf(ui.End).MaxWidthPercent(85).MinWidth(0).Gap(4).AlignItems(ui.End).Children(func() {
				ui.Column(c).MaxWidthPercent(100).MinWidth(0).Padding(10, 14).Radius(14).Background(bubbleColor(c)).Gap(6).Children(body)
				// The name stays readable for assistive technology and tests.
				kit.Caption(c, name+" · "+stamp)
			})
			return
		}
		ui.Row(c).FillWidth().MinWidth(0).Gap(10).AlignItems(ui.Start).Children(func() {
			nativeAvatar(c, name, 28)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, name).FontWeight(500).Shrink(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(name)
					kit.Caption(c, stamp).Shrink(0)
				})
				body()
			})
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

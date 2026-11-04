package flashchat

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/flashchat"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// Layout of the page, from interconnect.css: one fixed conversation column
// beside the conversation; below narrowWidth the list replaces the
// conversation when opened.
const (
	sidebarWidth  = 208
	narrowWidth   = 600
	hintWidth     = 760
	maxNameLength = 80
	lobbyWidth    = 400
)

// View builds the page.
func (p *page) View(c *ui.Context) {
	// The shell keeps the page, with its room and drafts, while the user
	// visits others (SetAside), so nothing here needs guarding.
	p.env.Activity.Report("flash-chat", false, false, nil)
	root := ui.Column(c).Fill().Bind(&p.area)
	width := p.area.Bounds(c).W
	root.Children(func() {
		if p.session == nil {
			p.lobby(c)
		} else {
			p.workspace(c, width)
		}
	})
	p.dialogs(c)
}

func i18nClose() string { return i18n.T("common.close") }

// textField is an input with a placeholder that names it, as the React
// page's fields, bounded to maxNameLength characters.
func textField(c *ui.Context, value *string, label string, disabled bool) ui.Element {
	in := ui.TextInput(c, value).FillWidth().Placeholder(label).Label(label).Disabled(disabled)
	if utf8.RuneCountInString(*value) > maxNameLength {
		*value = string([]rune(*value)[:maxNameLength])
	}
	return in
}

func (p *page) lobby(c *ui.Context) {
	t := c.Theme()
	// The welcome is centered as a whole, and scrolls when the window is
	// too low for it.
	// A scroll view does not stretch its content: the column takes the
	// frame's height from the last layout, less the padding.
	height := max(p.area.Bounds(c).H-64, 0)
	ui.Scroll(c).Fill().Padding(32, 24).Children(func() {
		ui.Column(c).FillWidth().MinHeight(height).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(lobbyWidth).Gap(24).Children(func() {
				ui.Column(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icons.MessagesCircle).FontSize(28).Shrink(0)
						kit.Title(c, tr("peerChat")).Role(ui.RoleHeading).Level(1)
					})
					kit.Help(c, tr("chatWelcome")).TextAlign(ui.Center)
				})
				if p.err != "" {
					kit.Notice(c, kit.Failure, "", p.err, func() {
						if !p.nickReady && kit.Button(c, tr("retry"), nil, false).Disabled(p.busy).Clicked() {
							p.err = ""
							p.loadNickname()
						}
					})
				}
				ui.Column(c).FillWidth().Gap(12).Children(func() {
					// A room's name is this side's own label for it; joining
					// needs only its code and password, and the name can be
					// set in the room.
					if !p.joining {
						textField(c, &p.roomName, tr("roomName"), p.busy)
					}
					p.nicknameField(c, false)
				})
				if !p.joining {
					ui.Column(c).FillWidth().Gap(12).Children(func() {
						if kit.PrimaryButton(c, tr("createRoom"), nil, p.busy).FillWidth().Justify(ui.Center).Disabled(p.busy || !p.nickReady).Clicked() {
							p.enter(true)
						}
						if kit.Button(c, tr("joinRoom"), nil, false).FillWidth().Justify(ui.Center).Disabled(p.busy).Clicked() {
							p.joining, p.err, p.roomName = true, "", ""
						}
					})
				} else {
					ui.Column(c).FillWidth().Gap(12).Children(func() {
						id := ui.TextInput(c, &p.roomID).FillWidth().Placeholder(tr("roomCode")).Label(tr("roomCode")).AutoFocus().Disabled(p.busy)
						pw := ui.TextInput(c, &p.password).FillWidth().Placeholder(tr("roomPassword")).Label(tr("roomPassword")).Disabled(p.busy)
						ready := p.nickReady && strings.TrimSpace(p.roomID) != "" && p.password != ""
						submit := id.Submitted() || pw.Submitted()
						if kit.PrimaryButton(c, tr("joinRoom"), nil, p.busy).FillWidth().Justify(ui.Center).Disabled(p.busy || !ready).Clicked() || (submit && ready && !p.busy) {
							p.enter(false)
						}
						if kit.GhostButton(c, tr("backToEntry"), nil, false).FillWidth().Justify(ui.Center).Disabled(p.busy).Clicked() {
							p.joining = false
						}
					})
				}
				ui.Column(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					if kit.GhostButton(c, tr("networkOptions"), icons.Settings2, false).Clicked() {
						p.panel = "network"
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icons.ShieldCheck).FontSize(15).TextColor(t.TextMuted).Shrink(0)
						kit.Caption(c, tr("chatEncryption"))
					})
				})
			})
		})
	})
}

// nicknameField edits the personal nickname; leaving it saves it, and in
// a room renames this side.
func (p *page) nicknameField(c *ui.Context, inRoom bool) {
	in := textField(c, &p.nickname, tr("nickname"), p.busy || !p.nickReady)
	focused := in.Focused()
	if p.nickFocused && !focused && p.nickReady {
		if inRoom {
			p.updateNickname()
		} else {
			nickname := p.nickname
			go func() {
				if err := p.saveNickname(nickname, true); err != nil {
					p.env.Post(func() { p.err = err.Error() })
				}
			}()
		}
	}
	p.nickFocused = focused
}

// groupName is the room's name, or the group conversation's.
func (p *page) groupName() string {
	if name := strings.TrimSpace(p.roomName); name != "" {
		return name
	}
	return tr("groupRoom")
}

func (p *page) workspace(c *ui.Context, width float32) {
	narrow := width > 0 && width < narrowWidth
	if !narrow {
		p.showPeers = false
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if !narrow || p.showPeers {
			p.sidebar(c, narrow)
		}
		if p.session != nil && (!narrow || !p.showPeers) {
			p.conversation(c, narrow, width)
		}
	})
}

// avatar is a peer's initial in a circle.
func avatar(c *ui.Context, name string) {
	initial, _ := utf8.DecodeRuneInString(name)
	pal := kit.Colors(c)
	ui.Box(c).Size(30, 30).Radius(15).Background(pal.Selected).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Text(c, string(initial)).FontWeight(500)
	})
}

// conversationButton is one entry of the conversation list.
func (p *page) conversationButton(c *ui.Context, chosen bool, label string, icon func(), name, detail string) ui.Element {
	t, pal := c.Theme(), kit.Colors(c)
	b := ui.ButtonBase(c).FillWidth().MinHeight(56).Padding(10).Gap(10).Radius(t.Radius).AlignItems(ui.Center).
		Label(label).Checked(chosen).Disabled(p.busy)
	switch {
	case chosen:
		b.Background(pal.Selected)
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	return b.Children(func() {
		icon()
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, name).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(name)
			kit.Caption(c, detail).SingleLine().Ellipsis("…").MinWidth(0)
		})
	})
}

func (p *page) sidebar(c *ui.Context, narrow bool) {
	t, pal := c.Theme(), kit.Colors(c)
	peers := p.session.Group().Peers()
	side := ui.Column(c).FillHeight().Background(pal.Sidebar).Label(tr("chatPeers")).Role(ui.RoleGroup)
	if narrow {
		side.Grow(1).MinWidth(0)
	} else {
		side.Width(sidebarWidth).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline)
	}
	side.Children(func() {
		ui.Row(c).FillWidth().MinHeight(72).Padding(16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.MessagesCircle).FontSize(20).Shrink(0)
			ui.Text(c, tr("peerChat")).FontSize(t.Rem(16.0 / 14)).FontWeight(500).Grow(1).SingleLine().MinWidth(0)
			if narrow && kit.IconButton(c, icons.X, tr("backToChat")).Clicked() {
				p.showPeers = false
			}
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 8, 16, 8).Gap(4).Label(tr("chatConversations")).Children(func() {
			name := p.groupName()
			if p.conversationButton(c, p.selected == "", tr("groupRoom"), func() {
				ui.Icon(c, icons.Users).FontSize(19).Shrink(0)
			}, name, tr("memberCount", "count", len(peers)+1)).Clicked() {
				p.selected, p.showPeers = "", false
			}
			ui.Text(c, tr("chatPeers")).FontSize(kit.HelpSize(c)).FontWeight(500).TextColor(t.TextMuted).Padding(20, 10, 6, 10).
				Role(ui.RoleHeading).Level(2)
			if len(peers) == 0 {
				kit.Help(c, tr("noPeers")).Padding(0, 10)
			}
			for _, peer := range peers {
				display := flashchat.PeerDisplayName(peer)
				state := "unverified"
				if peer.Verification == flashchat.Verified {
					state = "verified"
				}
				if p.conversationButton(c.Key(peer.PeerID), p.selected == peer.PeerID, tr("chatPrivate")+" · "+display, func() {
					avatar(c, display)
				}, display, tr(state)).Clicked() {
					p.selectPeer(peer.PeerID)
				}
			}
		})
		self := p.session.Nickname()
		ui.Row(c).FillWidth().Padding(16, 12).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
			avatar(c, self)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, self).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(self)
				kit.Caption(c, tr("chatYou"))
			})
			if kit.IconButton(c, icons.LogOut, tr("leaveRoom")).Disabled(p.busy).Clicked() {
				p.leave()
			}
		})
	})
}

// conversationPeer is the correspondent of a direct conversation, as its
// own room knows them (their typing in particular).
func (p *page) conversationPeer(room *flashchat.ChatRoom) *flashchat.Peer {
	for _, list := range [][]flashchat.Peer{room.Peers(), p.session.Group().Peers()} {
		for _, peer := range list {
			if peer.PeerID == p.selected {
				return &peer
			}
		}
	}
	return nil
}

func (p *page) conversation(c *ui.Context, narrow bool, width float32) {
	t, pal := c.Theme(), kit.Colors(c)
	room := p.room()
	if room == nil {
		p.selected = ""
		room = p.session.Group()
	}
	groupPeers := p.session.Group().Peers()
	title, subtitle := p.groupName(), tr("memberCount", "count", len(groupPeers)+1)
	typing := false
	if p.selected != "" {
		title, subtitle = tr("chatPrivate"), tr("chatPrivate")
		if peer := p.conversationPeer(room); peer != nil {
			title = flashchat.PeerDisplayName(*peer)
			typing = peer.TypingDirect
		}
	} else {
		for _, peer := range groupPeers {
			typing = typing || peer.TypingGroup
		}
	}
	gutter := float32(24)
	if narrow {
		gutter = 16
	}
	main := ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight()
	widgets.DropArea(c, p.env, main, widgets.DropFiles, true, p.busy, func(paths []string, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.offerDropped(paths)
	})
	main.Children(func() {
		ui.Row(c).FillWidth().MinHeight(72).Padding(12, gutter).Gap(12).AlignItems(ui.Center).Shrink(0).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
			if narrow && kit.IconButton(c, icons.Users, tr("chatPeers")).Clicked() {
				p.showPeers = true
			}
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, title).FontSize(t.Rem(16.0 / 14)).FontWeight(500).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(title).
					Role(ui.RoleHeading).Level(1)
				kit.Caption(c, subtitle)
			})
			if kit.IconButton(c, icons.Info, tr("roomDetails")).Clicked() {
				p.panel = "room"
			}
		})
		if p.unreachable {
			ui.Column(c).FillWidth().Padding(12, gutter, 0, gutter).Shrink(0).Children(func() {
				kit.Notice(c, kit.Caution, own("unreachableTitle"), own("unreachableHint"), func() {
					if kit.GhostButton(c, tr("networkOptions"), icons.Settings2, false).Clicked() {
						p.panel = "network"
					}
				})
			})
		}
		if p.err != "" {
			ui.Column(c).FillWidth().Padding(12, gutter, 0, gutter).Shrink(0).MaxHeightPercent(25).Children(func() {
				kit.Notice(c, kit.Failure, "", p.err, func() {
					if kit.GhostButton(c, i18nClose(), nil, false).Clicked() {
						p.err = ""
					}
				})
			})
		}
		p.transcript(c, room, groupPeers, gutter)
		p.offers(c, room, groupPeers, gutter)
		p.transfers(c, groupPeers, gutter)
		p.composer(c, room, typing, width, gutter)
	})
}

func (p *page) transcript(c *ui.Context, room *flashchat.ChatRoom, peers []flashchat.Peer, gutter float32) {
	t := c.Theme()
	messages := room.MessageLog()
	list := p.lists[p.selected]
	if list == nil {
		// A conversation opens at its latest message and follows new ones
		// while the reader is at the end.
		list = &ui.ListState{FollowEnd: true, FollowAnimation: &ui.Smooth}
		p.lists[p.selected] = list
	}
	list.Key = func(i int) any { return messages[i].AuthorID + "\x00" + messages[i].ID }
	if len(messages) == 0 {
		alone := len(room.Peers()) == 0 && len(peers) == 0
		height := p.empty.Bounds(c).H
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Bind(&p.empty).Children(func() {
			ui.Column(c).FillWidth().MinHeight(height).Justify(ui.Center).AlignItems(ui.Center).Padding(24, gutter).Gap(12).Children(func() {
				ui.Icon(c, icons.MessageCircle).FontSize(30).TextColor(t.TextMuted)
				heading, hint := "chatStart", "chatStartHint"
				if alone {
					heading, hint = "noPeers", "chatInviteHint"
				}
				ui.Text(c, tr(heading)).FontSize(t.Rem(16.0 / 14)).FontWeight(500).TextAlign(ui.Center)
				kit.Help(c, tr(hint)).TextAlign(ui.Center).MaxWidth(320)
				if alone {
					p.copyButton(c).Margin(8, 0, 0, 0)
				}
			})
		})
		return
	}
	self := p.session.UserID
	ui.List(c, list, len(messages), func(i int) {
		m := messages[i]
		mine := m.AuthorID == self
		name := m.AuthorID
		if mine {
			name = p.session.Nickname()
		} else {
			for _, peer := range peers {
				if peer.UserID == m.AuthorID {
					name = flashchat.PeerDisplayName(peer)
					break
				}
			}
		}
		align := ui.Start
		if mine {
			align = ui.End
		}
		ui.Column(c).FillWidth().AlignItems(align).MinWidth(0).Children(func() {
			ui.Row(c).Gap(8).Wrap().MaxWidthPercent(100).Margin(0, 0, 6, 0).Children(func() {
				kit.Caption(c, name).MinWidth(0)
				kit.Caption(c, time.UnixMilli(int64(m.TimeSent)).Format("15:04"))
			})
			body := ui.Column(c).MaxWidthPercent(100).MinWidth(0).Selectable()
			if mine {
				body.Padding(10, 14).Radius(12).Background(kit.Colors(c).Selected)
			}
			body.Children(func() {
				if m.Media {
					if kit.Button(c, tr("fileOffer"), icons.FolderOpen, false).Disabled(p.busy).Clicked() {
						p.receive(m.MagnetURI)
					}
					return
				}
				markdown.View(c, m.Text, &p.md)
			})
		})
	}).FillWidth().Grow(1).MinHeight(0).Gap(24).Padding(24, gutter).Label(tr("peerChat"))
}

// offers lists what the peers of the conversation offer.
func (p *page) offers(c *ui.Context, room *flashchat.ChatRoom, peers []flashchat.Peer, gutter float32) {
	offers := room.FileOffers()
	if len(offers) == 0 {
		return
	}
	t := c.Theme()
	ui.Scroll(c).FillWidth().Shrink(0).MaxHeightPercent(20).Children(func() {
		for peerID, offer := range offers {
			name := ""
			for _, peer := range peers {
				if peer.PeerID == peerID {
					name = flashchat.PeerDisplayName(peer)
				}
			}
			ui.Row(c.Key(peerID)).FillWidth().Padding(8, gutter).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons.FolderOpen).FontSize(16).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, tr("fileOffer")).MinWidth(0)
				if name != "" {
					kit.Caption(c, name).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
				}
				if kit.Button(c, tr("browseShare"), nil, false).Disabled(p.busy).Clicked() {
					p.receive(offer.MagnetURI)
				}
			})
		}
	})
}

// Opening and revealing a received file go to the system's shell, off the
// UI thread; tests replace them.
var (
	openPath     = func(path string) { go func() { _ = mygo.Shell.OpenPath(path) }() }
	revealInFold = func(path string) { go mygo.Shell.ShowItemInFolder(path) }
)

// transfers lists the files sent and received in the conversation, with
// their progress, and opens received ones.
func (p *page) transfers(c *ui.Context, peers []flashchat.Peer, gutter float32) {
	list := p.session.Transfers(p.selected)
	if len(list) == 0 {
		return
	}
	t := c.Theme()
	nameOf := func(id string) string {
		for _, peer := range peers {
			if peer.PeerID == id {
				return flashchat.PeerDisplayName(peer)
			}
		}
		return ""
	}
	ui.Scroll(c).FillWidth().Shrink(0).MaxHeightPercent(30).Label(own("filesTitle")).Children(func() {
		ui.Column(c).FillWidth().Padding(4, gutter).Gap(2).Children(func() {
			for _, f := range list {
				f := f
				ui.Row(c.Key("file-"+f.ID)).FillWidth().Padding(6, 0).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
					icon := icons.FileDown
					if f.Outgoing {
						icon = icons.FileUp
					}
					ui.Icon(c, icon).FontSize(18).TextColor(t.TextMuted).Shrink(0)
					ui.Column(c).Grow(1).Shrink(1).Basis(160).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, f.Name).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(f.Path)
						who := nameOf(f.PeerID)
						status := ""
						switch f.State {
						case flashchat.TransferSending, flashchat.TransferReceiving:
							percent := 100
							if f.Size > 0 {
								percent = int(f.Moved * 100 / f.Size)
							}
							key := "fileReceiving"
							if f.Outgoing {
								key = "fileSending"
							}
							status = own(key, "percent", percent)
						case flashchat.TransferDone:
							status = own(map[bool]string{true: "fileSent", false: "fileReceived"}[f.Outgoing])
						default:
							status = own("fileFailed", "error", f.Err)
						}
						line := fileSize(f.Size) + " · " + status
						if who != "" && !f.Outgoing {
							line = who + " · " + line
						}
						caption := kit.Caption(c, line).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(line)
						if f.State == flashchat.TransferFailed {
							caption.TextColor(t.Danger)
						}
						if f.State == flashchat.TransferSending || f.State == flashchat.TransferReceiving {
							value := float64(1)
							if f.Size > 0 {
								value = float64(f.Moved) / float64(f.Size)
							}
							ui.Progress(c, value).FillWidth().Label(f.Name)
						}
					})
					if f.State == flashchat.TransferDone && !f.Outgoing {
						if kit.Button(c, own("openFile"), nil, false).Clicked() {
							openPath(f.Path)
						}
						if kit.IconButton(c, icons.FolderOpen, own("showInFolder")).Clicked() {
							revealInFold(f.Path)
						}
					}
				})
			}
		})
	})
}

// fileSize writes a byte count in the largest unit that keeps it above 1.
func fileSize(n int64) string {
	value, units := float64(n), []string{"B", "KB", "MB", "GB"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func (p *page) composer(c *ui.Context, room *flashchat.ChatRoom, typing bool, width, gutter float32) {
	t := c.Theme()
	ui.Column(c).FillWidth().Shrink(0).Padding(0, gutter, 20, gutter).Children(func() {
		line := " "
		switch {
		case typing:
			line = tr("typing")
		case p.selected == "" && len(p.session.Group().Peers()) == 0:
			line = tr("chatSoloHint")
		}
		kit.Caption(c, line).MinHeight(28).Padding(4, 0).Role(ui.RoleStatus)
		box := ui.Column(c).FillWidth().Padding(10).Gap(8).Radius(kit.PanelRadius).Border(1, t.Border)
		box.Children(func() {
			selected := p.selected
			draft := p.drafts[selected]
			area := ui.TextAreaBase(c, &draft).FillWidth().Lines(3, 7).Padding(2, 4).LineHeight(1.65).Placeholder(tr("chatDraft")).Label(tr("chatDraft")).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key != ui.KeyEnter || k.Mods != 0 {
						return false
					}
					// Enter sends; Shift+Enter breaks the line.
					p.drafts[selected] = k.Text()
					if r := p.room(); r != nil && p.selected == selected && p.canSend(r) {
						p.send()
					}
					return true
				})
			if area.Focused() {
				box.Border(1, t.Focus)
			}
			if draft != p.drafts[selected] {
				p.drafts[selected] = draft
				room.NotifyTyping()
			}
			ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				if kit.IconButton(c, icons.FilePlus, tr("shareInChat")).Disabled(p.busy).Clicked() {
					p.shareFiles()
				}
				if room.SelfFileOffer() != "" && kit.GhostButton(c, tr("stopChatShare"), nil, false).Disabled(p.busy).Clicked() {
					p.stopSharing()
				}
				ui.Box(c).Grow(1)
				if width == 0 || width >= hintWidth {
					kit.Caption(c, tr("chatKeyboardHint"))
				}
				if kit.PrimaryButton(c, tr("sendMessage"), icons.Send, false).Disabled(!p.canSend(room)).Clicked() {
					p.send()
				}
			})
		})
	})
}

// codeBlock shows a message's fenced code with its language and a copy
// button.
func (p *page) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c).AlignSelf(ui.Stretch).Background(t.Surface).Border(1, pal.Hairline).Radius(kit.PanelRadius).Clip().
		Role(ui.RoleGroup).Label(b.Language).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(2, 2, 0, 12).Children(func() {
			kit.Caption(c, b.Language).Grow(1).MinWidth(0)
			if kit.IconButton(c, icons.Copy, i18n.T("chat.copyCode")).Disabled(b.Open).Clicked() {
				c.Services().WriteClipboard(b.Code())
				p.env.Toast(tr("copied"))
			}
		})
		b.Body(c)
	})
}

// copyButton copies the room id and password.
func (p *page) copyButton(c *ui.Context) ui.Element {
	label, icon := tr("roomCopy"), icons.Copy
	if p.copied {
		label, icon = tr("copied"), icons.Check
	}
	b := kit.Button(c, label, icon, false).Disabled(p.busy)
	if b.Clicked() {
		p.copyRoom(c.Services())
	}
	return b
}

func (p *page) dialogs(c *ui.Context) {
	if p.panel == "network" {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: tr("networkOptions"), Message: tr("chatNetworkHint"), Cancel: i18nClose(), Width: 520,
			Body: func() {
				field := func(value *string, label string) {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						kit.Label(c, label)
						ui.TextArea(c, value).FillWidth().Lines(4, 8).Label(label).Font(kit.MonoFont)
					})
				}
				field(&p.tracker, tr("tracker"))
				field(&p.rtc, tr("rtcConfig"))
			}})
		if !open {
			p.panel = ""
		}
	}
	if p.panel == "room" && p.session != nil {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: tr("roomDetails"), Message: tr("roomDetailsHint"), Cancel: i18nClose(), Width: 520,
			Body: func() {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.Label(c, tr("roomName"))
					textField(c, &p.roomName, tr("roomName"), false)
				})
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.Label(c, tr("nickname"))
					p.nicknameField(c, true)
				})
				readOnly := func(value, label string) {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						kit.Label(c, label)
						shown := value
						ui.TextInput(c, &shown).FillWidth().ReadOnly(true).Label(label).Font(kit.MonoFont)
					})
				}
				readOnly(p.session.RoomID, tr("roomCode"))
				readOnly(p.session.Password, tr("roomPassword"))
				ui.Row(c).Children(func() { p.copyButton(c) })
				kit.Help(c, tr("chatHistoryHint"))
				kit.Help(c, tr("chatHint"))
			},
			Actions: func() {
				if kit.Button(c, tr("leaveRoom"), icons.LogOut, false).Disabled(p.busy).Clicked() {
					p.leave()
				}
			}})
		if !open && p.panel == "room" {
			p.panel = ""
		}
	} else if p.panel == "room" {
		p.panel = ""
	}
	if p.browsing && p.browse != nil {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: tr("browseShare"), Cancel: i18nClose(), Width: 1000, Body: func() {
			p.browse.View(c, false).Radius(kit.PanelRadius).Border(1, c.Theme().Border)
		}})
		if !open {
			p.closeBrowse()
		}
	}
}

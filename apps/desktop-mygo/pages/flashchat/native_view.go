package flashchat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type nativePeer struct {
	AudioChannels map[string]string `json:"audioChannels"`
	PeerID        string            `json:"peerId"`
	UserID        string            `json:"userId"`
	Name          string            `json:"name"`
	Verified      int               `json:"verified"`
	OfferedFileID string            `json:"offeredFileId"`
	Typing        bool              `json:"typing"`
	DirectTyping  bool              `json:"directTyping"`
	Connection    string            `json:"connection"`
}
type nativeMessage struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	AuthorID string `json:"authorId"`
	TimeSent int64  `json:"timeSent"`
	Magnet   string `json:"magnetURI"`
}
type nativeRoom struct {
	Unread   int             `json:"unread"`
	Messages []nativeMessage `json:"messages"`
	Sending  bool            `json:"sending"`
	Offer    string          `json:"offer"`
}
type nativeSettings struct {
	Nickname      string          `json:"customUsername"`
	Sound         bool            `json:"playSoundOnNewMessage"`
	Notification  bool            `json:"showNotificationOnNewMessage"`
	Typing        bool            `json:"showActiveTypingStatus"`
	Enhanced      bool            `json:"isEnhancedConnectivityEnabled"`
	SelectedSound string          `json:"selectedSound"`
	Trackers      []string        `json:"trackerUrls"`
	RTC           json.RawMessage `json:"rtcConfig"`
}
type nativeStream struct{ ID, Peer, Kind string }
type nativeDevice struct{ ID, Label string }
type nativePreview struct {
	ID, Name, Kind    string
	Duration, Current float64
	Playing           bool
}
type nativeSnapshot struct {
	EnhancedAvailable bool                            `json:"enhancedAvailable"`
	Previews          map[string][]nativePreview      `json:"previews"`
	Revision          int                             `json:"revision"`
	Ready             bool                            `json:"ready"`
	UserID            string                          `json:"userId"`
	RoomID            string                          `json:"roomId"`
	Secret            string                          `json:"secret"`
	Settings          nativeSettings                  `json:"settings"`
	Peers             []nativePeer                    `json:"peers"`
	Rooms             map[string]nativeRoom           `json:"rooms"`
	Alert             string                          `json:"alert"`
	Community         []string                        `json:"community"`
	Sounds            []struct{ Label, Value string } `json:"sounds"`
	Media             struct {
		Microphone, Camera, Screen bool
		AudioDevices, VideoDevices []nativeDevice
		Streams                    []nativeStream
	} `json:"media"`
	Transfer struct {
		Busy     bool
		Progress float64
	} `json:"transfer"`
	Connections struct {
		HasHost, HasTURNServer bool
		TrackerConnection      string
	} `json:"connections"`
}
type nativePageState struct {
	presentation                                             string
	background                                               []nativeAction
	previewRequested                                         map[string]bool
	previewErrors                                            map[string]string
	previewID, qrLink                                        string
	qr                                                       *ui.Bitmap
	snapshot                                                 nativeSnapshot
	bridge                                                   *nativeBridge
	busy                                                     bool
	downloading                                              bool
	actionError                                              string
	formRoom, formNickname, formPassword, roomKind, nameKind string
	selected, panel, settingsTab, keyText, identityName      string
	trackers, rtc, embedRoot                                 string
	showPeers, hideMessages, pushTalking                     bool
	dismissedAlert                                           string
	drafts                                                   map[string]string
	lists                                                    map[string]*ui.ListState
	frames                                                   map[string]*ui.Bitmap
	volumes                                                  map[string]float64
	area                                                     ui.Handle
	md                                                       markdown.Options
}

func nt(key string, args ...any) string { return i18n.T("flashChatNative."+key, args...) }
func (p *completePage) initNative() {
	p.previewRequested, p.previewErrors = map[string]bool{}, map[string]string{}
	p.roomKind, p.nameKind = "public", "id"
	p.drafts, p.lists = map[string]string{}, map[string]*ui.ListState{}
	p.frames, p.volumes = map[string]*ui.Bitmap{}, map[string]float64{}
	p.md = markdown.Options{WrapCode: true, CodeBlock: p.nativeCode}
}

func (p *completePage) nativeView(c *ui.Context) {
	root := ui.Column(c).Fill().MinWidth(0).MinHeight(0).Bind(&p.area)
	root.Children(func() {
		if p.snapshot.RoomID == "" {
			p.nativeLobby(c)
		} else {
			p.nativeWorkspace(c)
		}
	})
	p.nativeDialogs(c)
}

func (p *completePage) feedback(c *ui.Context) {
	message := p.err
	if message == "" {
		message = p.actionError
	}
	if message != "" {
		kit.Notice(c, kit.Failure, "", message, func() {
			if kit.GhostButton(c, tr("retry"), nil, false).Clicked() {
				p.err, p.actionError = "", ""
				if p.view != nil && !p.snapshot.Ready {
					p.snapshot = nativeSnapshot{}
					p.view.Page().Reload()
				}
			}
		})
	}
	if alert := p.snapshot.Alert; alert != "" && alert != p.dismissedAlert {
		kit.Notice(c, kit.Info, "", alert, func() {
			if kit.GhostButton(c, i18nClose(), nil, false).Clicked() {
				p.dismissedAlert = alert
			}
		})
	}
}

func nativeField(c *ui.Context, label string, value *string, password bool) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		kit.Label(c, label)
		in := ui.TextInput(c, value).Label(label).FillWidth().Height(40)
		if password {
			in.Password()
		}
	})
}

func (p *completePage) nativeConversation(c *ui.Context) {
	room := p.snapshot.Rooms[p.selected]
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
		ui.Column(c).FillWidth().Padding(12, 20, 0, 20).Gap(8).Children(func() { p.feedback(c) })
		p.nativeMedia(c)
		if !p.hideMessages || len(p.snapshot.Media.Streams) == 0 {
			if len(room.Messages) == 0 {
				ui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(24).Gap(10).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Icon(c, icons.MessagesCircle).FontSize(30).TextColor(c.Theme().TextMuted)
					kit.Heading(c, nt("startTalking"))
					kit.Help(c, nt("inviteHint")).TextAlign(ui.Center)
					if kit.Button(c, nt("invite"), icons.Share2, false).Clicked() {
						p.panel = "invite"
					}
				})
			} else {
				list := p.lists[p.selected]
				if list == nil {
					list = &ui.ListState{FollowEnd: true}
					p.lists[p.selected] = list
				}
				list.Key = func(i int) any { return room.Messages[i].AuthorID + room.Messages[i].ID }
				ui.List(c, list, len(room.Messages), func(i int) { p.nativeMessage(c, room.Messages[i]) }).FillWidth().Grow(1).MinHeight(0).Gap(24).Padding(24).Label(nt("messages"))
			}
		}
		p.nativeComposer(c, room)
	})
}

func (p *completePage) nativeCode(c *ui.Context, block *markdown.CodeBlockView) {
	ui.Column(c).FillWidth().MinWidth(0).Background(c.Theme().Surface).Border(1, kit.Colors(c).Hairline).Radius(8).Children(func() {
		ui.Row(c).FillWidth().Padding(4, 10).AlignItems(ui.Center).Children(func() {
			kit.Caption(c, block.Language).Grow(1)
			if kit.IconButton(c, icons.Copy, nt("copyCode")).Clicked() {
				c.Services().WriteClipboard(block.Code())
			}
		})
		block.Body(c)
	})
}

func (p *completePage) nativeComposer(c *ui.Context, room nativeRoom) {
	ui.Column(c).FillWidth().Shrink(0).Padding(0, 24, 20, 24).Gap(8).Children(func() {
		var typing []string
		for _, peer := range p.snapshot.Peers {
			if p.snapshot.Settings.Typing && ((p.selected == "" && peer.Typing) || (p.selected == peer.PeerID && peer.DirectTyping)) {
				typing = append(typing, peer.Name)
			}
		}
		label := ""
		if len(typing) > 0 {
			label = nt("typing", "names", strings.Join(typing, ", "))
		}
		if label != "" {
			kit.Caption(c, label)
		}
		selected := p.selected
		draft := p.drafts[selected]
		ui.Column(c).FillWidth().Border(1, c.Theme().Border).Radius(12).Background(c.Theme().Surface).Padding(8).Gap(4).Children(func() {
			area := ui.TextArea(c.Key(selected), &draft).FillWidth().Lines(2, 7).BorderWidth(0).Background(ui.RGBA(0, 0, 0, 0)).Label(tr("chatDraft")).Placeholder(tr("chatDraft")).OnTextKey(func(key ui.TextKey) bool {
				if key.Key != ui.KeyEnter || key.Mods != 0 {
					return false
				}
				p.drafts[selected] = key.Text()
				p.sendNative()
				return true
			})
			widgets.DropArea(c, p.env, area, widgets.DropFiles, true, p.busy, func(paths []string, err error) {
				if err != nil {
					p.actionError = err.Error()
				} else {
					p.shareNative(paths)
				}
			})
			if draft != p.drafts[selected] {
				p.drafts[selected] = draft
				if p.snapshot.Settings.Typing && p.view != nil {
					p.backgroundCommand("room:"+selected, "typing", nil)
				}
			}
			ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				if kit.IconButton(c, icons.FilePlus, tr("shareInChat")).Disabled(p.busy || p.env.Dialogs == nil).Clicked() {
					p.shareNative(nil)
				}
				if room.Offer != "" && kit.GhostButton(c, tr("stopChatShare"), nil, false).Disabled(p.busy).Clicked() {
					p.command("room:"+selected, "rescind", nil, nil)
				}
				ui.Box(c).Grow(1)
				ptt := kit.IconButton(c, icons.Mic, nt("pushToTalk")).Disabled(p.busy)
				pressed := ptt.Pressed() || (c.Modifiers()&ui.Ctrl != 0 && c.KeyDown(ui.KeyBackquote))
				if pressed != p.pushTalking {
					p.pushTalking = pressed
					p.backgroundCommand("media", "microphone", map[string]any{"enabled": pressed})
				}
				if kit.PrimaryButton(c, tr("sendMessage"), icons.Send, room.Sending).Disabled(p.busy || room.Sending || strings.TrimSpace(draft) == "").Clicked() {
					p.sendNative()
				}
			})
		})
		kit.Caption(c, nt("keyboard")).AlignSelf(ui.End)
		if p.snapshot.Transfer.Busy {
			kit.Caption(c, nt("receiving", "percent", fmt.Sprintf("%.0f", p.snapshot.Transfer.Progress*100)))
		}
	})
}

func (p *completePage) sendNative() {
	selected, draft := p.selected, p.drafts[p.selected]
	if p.busy || strings.TrimSpace(draft) == "" {
		return
	}
	p.command("room:"+selected, "send", map[string]any{"text": draft}, func(json.RawMessage) {
		if p.drafts[selected] == draft {
			p.drafts[selected] = ""
		}
	})
}

func (p *completePage) nativeMedia(c *ui.Context) {
	media := p.snapshot.Media
	if len(media.Streams) == 0 {
		return
	}
	mediaArea := ui.Scroll(c).FillWidth().MinHeight(100)
	if p.hideMessages {
		mediaArea.Grow(1).MinHeight(0)
	} else {
		mediaArea.Height(240).MaxHeightPercent(40).Shrink(1)
	}
	mediaArea.Children(func() {
		ui.Row(c).FillWidth().Gap(10).Wrap().Padding(0, 20, 10, 20).Children(func() {
			for _, stream := range media.Streams {
				ui.Column(c.Key(stream.ID)).Grow(1).Basis(240).MinWidth(0).MaxWidthPercent(100).Gap(4).Children(func() {
					name := nt("you")
					for _, peer := range p.snapshot.Peers {
						if peer.PeerID == stream.Peer {
							name = peer.Name
						}
					}
					kit.Caption(c, name+" · "+nt(stream.Kind))
					if frame := p.frames[stream.ID]; frame != nil {
						if ui.ButtonBase(c).FillWidth().MinWidth(0).Label(nt("enlarge") + " · " + name).Children(func() { ui.Image(c, frame).FillWidth().Height(170).Fit(ui.Contain) }).Clicked() {
							p.previewID, p.panel = stream.ID, "preview"
						}
					} else {
						kit.Help(c, nt("mediaLoading"))
					}
				})
			}
		})
	})
}

func (p *completePage) invitation() string {
	kind := "public"
	if p.snapshot.Secret != "" {
		kind = "private"
	}
	link := "winone-flash://" + kind + "/" + url.PathEscape(p.snapshot.RoomID)
	if p.snapshot.Secret != "" {
		link += "#secret=" + url.QueryEscape(p.snapshot.Secret)
	}
	return link
}

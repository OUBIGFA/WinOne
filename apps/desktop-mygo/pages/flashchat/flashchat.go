// Package flashchat is the Flash Chat page: rooms of peers who talk over
// encrypted WebRTC, compatible with the Chitchatter rooms of the React
// window and its browser peers (package winone/desktopmygo/flashchat).
//
// React source: interconnect-chat-entry.tsx (the page),
// features/interconnect-chat-frame.tsx (its Host side: nickname, files,
// shares, drops, activity) and interconnect-chat-preferences.ts.
package flashchat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/core"
	"winone/desktopmygo/flashchat"
	"winone/desktopmygo/flashchat/trystero"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/web"
)

func init() {
	shell.Register("flash-chat", core.Conversation, func(env *core.Env) core.Page { return newPage(env) })
}

func tr(key string, args ...any) string { return i18n.T("interconnect."+key, args...) }

// own is the page's own text, beyond what the React window had.
func own(key string, args ...any) string { return i18n.T("flashChat."+key, args...) }

// joinFunc joins a room; tests replace it to use an in-memory network.
type joinFunc func(flashchat.JoinOptions) (*flashchat.Session, error)

type page struct {
	env  *core.Env
	join joinFunc
	md   markdown.Options

	// The lobby.
	roomName, roomID, password string
	nickname                   string
	nickReady                  bool
	joining                    bool // the join form rather than create
	tracker, rtc               string
	panel                      string // "network", "room" or ""

	// The room.
	session   *flashchat.Session
	selected  string // the conversation: "" the group, else a peer id
	drafts    map[string]string
	lists     map[string]*ui.ListState
	showPeers bool
	copied    bool
	copyTimer *time.Timer
	browse    *web.Surface
	browsing  bool
	revision  int

	busy bool
	err  string
	// unreachable tells that this computer offers peers on other networks
	// no address (ProbeReach), as when a proxy's virtual adapter takes
	// the UDP traffic WebRTC needs.
	unreachable bool
	// area is the page's frame, whose width chooses the narrow layout;
	// nickFocused saves the nickname when its field loses the focus.
	area, empty ui.Handle
	nickFocused bool

	tasks     *tasks
	shares    *shares
	receivers map[string]string // share address → receiving task
	// stopped is set while no room is wanted: after leaving, or once the
	// page closed. Waits for shares give up on it.
	stopped atomic.Bool
	closed  bool

	// saveMu orders nickname saves; saved is the nickname the Host has.
	saveMu  sync.Mutex
	savedMu sync.Mutex
	saved   string
}

func newPage(env *core.Env) *page {
	p := &page{env: env, join: flashchat.Join, drafts: map[string]string{}, lists: map[string]*ui.ListState{},
		tasks: newTasks(env), receivers: map[string]string{}}
	// Code wraps rather than scroll sideways; its copy button speaks the
	// window's language.
	p.md = markdown.Options{WrapCode: true, CodeBlock: p.codeBlock}
	p.loadNickname()
	return p
}

type preferences struct {
	Nickname string `json:"nickname"`
}

// loadNickname reads the personal nickname the Host keeps; a failure
// leaves the saved one alone and offers to read again.
func (p *page) loadNickname() {
	core.Go(p.env, func() (preferences, error) {
		var prefs preferences
		err := p.env.Host.CallCompletingInto("interconnect.chat.preferences.get", nil, &prefs)
		return prefs, err
	}, func(prefs preferences, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.savedMu.Lock()
		p.saved = prefs.Nickname
		p.savedMu.Unlock()
		p.nickname, p.nickReady = prefs.Nickname, true
	})
}

// saveNickname writes the nickname when it changed, one save at a time,
// so an older write never replaces a later one. It blocks.
func (p *page) saveNickname(value string, ready bool) error {
	p.saveMu.Lock()
	defer p.saveMu.Unlock()
	if !ready {
		return errors.New(own("nicknameNotLoaded"))
	}
	normalized := strings.TrimSpace(value)
	p.savedMu.Lock()
	same := p.saved == normalized
	p.savedMu.Unlock()
	if same {
		return nil
	}
	var result preferences
	if err := p.env.Host.CallCompletingInto("interconnect.chat.preferences.save", preferences{Nickname: normalized}, &result); err != nil {
		return err
	}
	p.savedMu.Lock()
	p.saved = result.Nickname
	p.savedMu.Unlock()
	return nil
}

// action runs work in the background as the page's one operation: the
// page is busy meanwhile and shows what failed.
func (p *page) action(work func() error, done func()) {
	p.busy, p.err = true, ""
	core.Do(p.env, work, func(err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
		}
		if done != nil {
			done()
		}
	})
}

// joinOptions validates the network settings as the React page did.
func (p *page) joinOptions(create bool) (flashchat.JoinOptions, error) {
	o := flashchat.JoinOptions{Nickname: p.nickname}
	if !create {
		o.RoomID, o.Password = strings.TrimSpace(p.roomID), p.password
		if o.RoomID == "" || o.Password == "" {
			return o, errors.New(tr("roomPassword"))
		}
	}
	if text := strings.TrimSpace(p.rtc); text != "" {
		rtc, err := trystero.ParseRTCConfig(text)
		if err != nil {
			return o, errors.New(own("invalidRtc", "error", err.Error()))
		}
		o.RTC = rtc
	}
	for _, line := range strings.Split(p.tracker, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if !trystero.ValidRelayURL(line) {
			return o, errors.New(own("invalidTracker", "url", line))
		}
		o.Trackers = append(o.Trackers, line)
	}
	return o, nil
}

// enter creates a room or joins one. The personal nickname is saved;
// room names and passwords never are.
func (p *page) enter(create bool) {
	o, err := p.joinOptions(create)
	if err != nil {
		p.err = err.Error()
		return
	}
	nickname, ready := p.nickname, p.nickReady
	p.stopped.Store(false)
	p.shares = newShares(p.tasks)
	o.Files = p.shares
	o.ReceiveDir = p.receiveDir
	o.OnEvent = func(_ *flashchat.ChatRoom, e flashchat.Event) {
		p.env.Post(func() {
			p.revision++
			if e.Kind == flashchat.Error && p.session != nil {
				p.err = roomError(e)
			}
		})
	}
	var session *flashchat.Session
	p.action(func() error {
		if err := p.saveNickname(nickname, ready); err != nil {
			return err
		}
		s, err := p.join(o)
		session = s
		return err
	}, func() {
		if session == nil {
			return
		}
		if p.closed {
			go session.Leave()
			return
		}
		p.session, p.selected = session, ""
		p.roomID, p.password = session.RoomID, session.Password
		p.unreachable = false
		rtc := o.RTC
		core.Go(p.env, func() (trystero.Reach, error) { return probeReach(rtc, 8*time.Second) }, func(r trystero.Reach, err error) {
			if p.session == session {
				p.unreachable = err == nil && !r.Public && !r.Relayed
			}
		})
	})
}

// probeReach checks this computer's reach; tests stay off the network.
var probeReach = trystero.ProbeReach

// roomError words a failure the room reported.
func roomError(e flashchat.Event) string {
	switch e.Context {
	case flashchat.ContextStopSeeding, flashchat.ContextRescindFileOffer, flashchat.ContextLeave:
		return own("stopShareFailed", "error", e.Err.Error())
	case flashchat.ContextSendMessage, flashchat.ContextSendInlineMedia:
		return tr("sendNotDelivered")
	}
	return own("peerError", "error", e.Err.Error())
}

// leave leaves the room at once on screen, then on the network, and
// stops the shares the room was receiving.
//
// It happens after the frame that asked for it: the room's views later in
// that frame still read the session, and leaving at once crashed the
// window instead of returning to the lobby.
func (p *page) leave() {
	p.env.Post(p.leaveNow)
}

func (p *page) leaveNow() {
	session := p.session
	p.stopped.Store(true)
	p.session, p.selected, p.panel, p.showPeers, p.joining = nil, "", "", false, false
	p.closeBrowse()
	if session == nil {
		return
	}
	receivers := p.receivers
	p.receivers = map[string]string{}
	p.action(func() error {
		session.Leave()
		for _, id := range receivers {
			_ = p.tasks.stop(id)
		}
		return nil
	}, nil)
}

// room is the conversation shown: the group room, or the direct one.
func (p *page) room() *flashchat.ChatRoom {
	if p.session == nil {
		return nil
	}
	return p.session.Existing(p.selected)
}

// selectPeer opens a conversation, which already listens: private
// messages do not wait for it to be opened.
func (p *page) selectPeer(peerID string) {
	session := p.session
	p.action(func() error {
		session.Direct(peerID)
		return nil
	}, func() {
		if p.session == session {
			p.selected, p.showPeers = peerID, false
		}
	})
}

// updateNickname saves the nickname and renames this side in the room.
func (p *page) updateNickname() {
	nickname, ready, session := p.nickname, p.nickReady, p.session
	core.Do(p.env, func() error {
		if err := p.saveNickname(nickname, ready); err != nil {
			return err
		}
		if session != nil {
			session.SetNickname(nickname)
		}
		return nil
	}, func(err error) {
		if err != nil {
			p.err = err.Error()
		}
		p.revision++
	})
}

// send posts the conversation's draft; it stays until delivered, and is
// cleared only if it was not edited meanwhile.
func (p *page) send() {
	room := p.room()
	target, content := p.selected, p.drafts[p.selected]
	if room == nil || !p.canSend(room) {
		return
	}
	p.action(func() error {
		if err := room.SendMessage(content); err != nil {
			return errors.New(tr("sendNotDelivered"))
		}
		return nil
	}, func() {
		if p.err == "" && p.drafts[target] == content {
			p.drafts[target] = ""
		}
		if list := p.lists[target]; list != nil && p.err == "" {
			list.ScrollToEnd()
		}
	})
}

func (p *page) canSend(room *flashchat.ChatRoom) bool {
	if p.busy || room == nil || strings.TrimSpace(p.drafts[p.selected]) == "" {
		return false
	}
	return p.selected == "" || len(room.Peers()) > 0
}

// files turns paths into the files of an offer, through the Host.
func (p *page) files(paths []string) ([]flashchat.FileHandle, error) {
	var files []flashchat.FileHandle
	err := p.env.Host.CallCompletingInto("interconnect.transfer_files", map[string]any{"paths": paths}, &files)
	return files, err
}

// shareFiles chooses files and sends them to the conversation shown,
// straight over the room's connections.
func (p *page) shareFiles() {
	session, peer := p.session, p.selected
	if session == nil {
		return
	}
	p.action(func() error {
		paths, err := p.env.Dialogs.PickFiles(tr("shareInChat"), core.Filter{})
		if err != nil || len(paths) == 0 {
			return err
		}
		return sendAll(session, paths, peer)
	}, nil)
}

// offerDropped sends files dragged onto the conversation.
func (p *page) offerDropped(paths []string) {
	session, peer := p.session, p.selected
	if session == nil {
		return
	}
	p.action(func() error { return sendAll(session, paths, peer) }, nil)
}

// sendAll sends each file; the first that cannot go stops the rest.
func sendAll(session *flashchat.Session, paths []string, peer string) error {
	for _, path := range paths {
		if err := session.SendFile(path, peer); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

// receiveDir is where files members send are saved: the folder chosen in
// the interconnect settings, or Downloads.
func (p *page) receiveDir() (string, error) {
	var general struct {
		DownloadDir string `json:"downloadDir"`
	}
	if err := p.env.Host.CallCompletingInto("interconnect.general.get", nil, &general); err == nil && filepath.IsAbs(general.DownloadDir) {
		return general.DownloadDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

func (p *page) stopSharing() {
	room := p.room()
	if room == nil {
		return
	}
	p.action(func() error {
		room.RescindFileOffer()
		return nil
	}, nil)
}

// receive connects to a peer's share and shows its page, served on this
// computer.
func (p *page) receive(code string) {
	id := p.receivers[code]
	p.action(func() error {
		var err error
		if id == "" {
			if id, err = p.tasks.start(map[string]any{"mode": "share_client", "code": code, "listen": "127.0.0.1:0"}); err != nil {
				return err
			}
		}
		r, err := p.tasks.ready(id, p.stopped.Load)
		if err != nil {
			_ = p.tasks.stop(id)
			id = ""
			return err
		}
		p.env.Post(func() { p.openBrowse("http://" + r.State.Address) })
		return nil
	}, func() {
		if id == "" {
			delete(p.receivers, code)
		} else {
			p.receivers[code] = id
		}
	})
}

func (p *page) openBrowse(address string) {
	if p.session == nil {
		return
	}
	if p.browse != nil && p.browse.URL() != address {
		p.closeBrowse()
	}
	if p.browse == nil {
		// Its files download to the Downloads folder, as the iframe's did.
		p.browse = web.New(p.env, web.Options{URL: address, Label: tr("browseShare"), Height: 560, Downloads: true})
	}
	p.browsing = true
}

func (p *page) closeBrowse() {
	if p.browse != nil {
		p.browse.Close()
		p.browse = nil
	}
	p.browsing = false
}

// copyRoom puts the room id and password on the clipboard.
func (p *page) copyRoom(services ui.Services) {
	if p.session == nil {
		return
	}
	services.WriteClipboard(p.session.RoomID + "\n" + p.session.Password)
	p.copied = true
	if p.copyTimer != nil {
		p.copyTimer.Stop()
	}
	p.copyTimer = time.AfterFunc(2*time.Second, func() { p.env.Post(func() { p.copied = false }) })
}

// SetAside implements core.Keeper: the room, its shares and the drafts go
// on while the user visits other pages; only the browsed share, a web view
// on screen, closes.
func (p *page) SetAside() {
	p.closeBrowse()
}

// closeTimeout bounds how long closing waits for the goodbye and the
// stopped shares.
const closeTimeout = 3 * time.Second

// Close leaves the room and stops every task the page started, as the
// React frame did on unmount: the shares only work while one is in the
// room. It waits a moment so that this happens before the window exits.
func (p *page) Close() {
	p.closed = true
	p.stopped.Store(true)
	p.env.Activity.Report("flash-chat", false, false, nil)
	if p.copyTimer != nil {
		p.copyTimer.Stop()
	}
	p.closeBrowse()
	session := p.session
	p.session = nil
	done := make(chan struct{})
	go func() {
		defer close(done)
		if session != nil {
			session.Leave()
		}
		p.tasks.stopAll()
	}()
	select {
	case <-done:
	case <-time.After(closeTimeout):
	}
}

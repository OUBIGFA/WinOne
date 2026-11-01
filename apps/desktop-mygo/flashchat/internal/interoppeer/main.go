// Command interoppeer is a headless WinOne flash chat peer for the
// interoperability test with the React/Chitchatter page
// (tests/flash-chat-interop). It reads one JSON command per line on stdin
// and writes one JSON result per line on stdout. It also runs the local
// test tracker, so the test needs no public service.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"winone/desktopmygo/flashchat"
	"winone/desktopmygo/flashchat/trystero"
	"winone/desktopmygo/flashchat/trystero/trackertest"
)

type command struct {
	Cmd      string                 `json:"cmd"`
	RoomID   string                 `json:"roomId"`
	Password string                 `json:"password"`
	Nickname string                 `json:"nickname"`
	Trackers []string               `json:"trackers"`
	RTC      string                 `json:"rtc"`
	Text     string                 `json:"text"`
	Peer     string                 `json:"peer"`
	Typing   bool                   `json:"typing"`
	Files    []flashchat.FileHandle `json:"files"`
}

// files stands in for the Host's shares: an offer gets a made-up address.
type files struct {
	mu        sync.Mutex
	n         int
	offering  map[string]bool
	rescinded []string
}

func (f *files) Offer([]flashchat.FileHandle, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	code := fmt.Sprintf("tc-interop-%d", f.n)
	f.offering[code] = true
	return code, nil
}

func (f *files) Rescind(code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.offering, code)
	f.rescinded = append(f.rescinded, code)
	return nil
}

func (f *files) RescindAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for code := range f.offering {
		f.rescinded = append(f.rescinded, code)
	}
	f.offering = map[string]bool{}
	return nil
}

func (f *files) IsOffering(code string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.offering[code]
}

type peerView struct {
	PeerID       string `json:"peerId"`
	UserID       string `json:"userId"`
	Name         string `json:"name"`
	Display      string `json:"display"`
	Verified     bool   `json:"verified"`
	TypingGroup  bool   `json:"typingGroup"`
	TypingDirect bool   `json:"typingDirect"`
}

type messageView struct {
	Author   string `json:"author"`
	Text     string `json:"text"`
	Media    bool   `json:"media"`
	Magnet   string `json:"magnet,omitempty"`
	Received bool   `json:"received"`
}

func messages(room *flashchat.ChatRoom) []messageView {
	views := []messageView{}
	if room == nil {
		return views
	}
	for _, m := range room.MessageLog() {
		views = append(views, messageView{Author: m.AuthorID, Text: m.Text, Media: m.Media, Magnet: m.MagnetURI, Received: m.Received})
	}
	return views
}

func main() {
	out := json.NewEncoder(os.Stdout)
	var outMu sync.Mutex
	reply := func(v map[string]any) {
		outMu.Lock()
		defer outMu.Unlock()
		_ = out.Encode(v)
	}
	var session *flashchat.Session
	shares := &files{offering: map[string]bool{}}
	var errs []string
	var errMu sync.Mutex
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		var c command
		if err := json.Unmarshal(in.Bytes(), &c); err != nil {
			reply(map[string]any{"error": err.Error()})
			continue
		}
		fail := func(err error) { reply(map[string]any{"cmd": c.Cmd, "error": err.Error()}) }
		peerID := func() string {
			if session == nil {
				return ""
			}
			for _, p := range session.Group().Peers() {
				if p.PeerID == c.Peer || p.CustomUsername == c.Peer {
					return p.PeerID
				}
			}
			return c.Peer
		}
		switch c.Cmd {
		case "tracker":
			t, err := trackertest.Start()
			if err != nil {
				fail(err)
				continue
			}
			reply(map[string]any{"cmd": c.Cmd, "url": t.URL})
		case "join":
			var rtc *trystero.RTCConfig
			if c.RTC != "" {
				var err error
				if rtc, err = trystero.ParseRTCConfig(c.RTC); err != nil {
					fail(err)
					continue
				}
			}
			s, err := flashchat.Join(flashchat.JoinOptions{RoomID: c.RoomID, Password: c.Password, Nickname: c.Nickname, Trackers: c.Trackers, RTC: rtc,
				Files: shares, Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
				OnEvent: func(_ *flashchat.ChatRoom, e flashchat.Event) {
					if e.Kind == flashchat.Error {
						errMu.Lock()
						errs = append(errs, string(e.Context)+": "+e.Err.Error())
						errMu.Unlock()
					}
				}})
			if err != nil {
				fail(err)
				continue
			}
			session = s
			reply(map[string]any{"cmd": c.Cmd, "roomId": s.RoomID, "password": s.Password, "userId": s.UserID})
		case "send":
			if err := session.Group().SendMessage(c.Text); err != nil {
				fail(err)
				continue
			}
			reply(map[string]any{"cmd": c.Cmd})
		case "dm":
			if err := session.Direct(peerID()).SendMessage(c.Text); err != nil {
				fail(err)
				continue
			}
			reply(map[string]any{"cmd": c.Cmd})
		case "typing":
			room := session.Group()
			if c.Peer != "" {
				room = session.Direct(peerID())
			}
			room.NotifyTyping()
			reply(map[string]any{"cmd": c.Cmd})
		case "offer":
			code, err := session.Group().OfferFiles(c.Files)
			if err != nil {
				fail(err)
				continue
			}
			reply(map[string]any{"cmd": c.Cmd, "code": code})
		case "rescind":
			session.Group().RescindFileOffer()
			reply(map[string]any{"cmd": c.Cmd})
		case "nickname":
			session.SetNickname(c.Nickname)
			reply(map[string]any{"cmd": c.Cmd})
		case "state":
			if session == nil {
				reply(map[string]any{"cmd": c.Cmd})
				continue
			}
			var peers []peerView
			dms := map[string][]messageView{}
			for _, p := range session.Group().Peers() {
				view := peerView{PeerID: p.PeerID, UserID: p.UserID, Name: p.CustomUsername, Display: flashchat.PeerDisplayName(p), Verified: p.Verification == flashchat.Verified, TypingGroup: p.TypingGroup}
				if dm := session.Existing(p.PeerID); dm != nil {
					dms[p.PeerID] = messages(dm)
					for _, q := range dm.Peers() {
						if q.PeerID == p.PeerID {
							view.TypingDirect = q.TypingDirect
						}
					}
				}
				peers = append(peers, view)
			}
			shares.mu.Lock()
			rescinded := append([]string(nil), shares.rescinded...)
			shares.mu.Unlock()
			errMu.Lock()
			errors := append([]string(nil), errs...)
			errMu.Unlock()
			reply(map[string]any{"cmd": c.Cmd, "peers": peers, "group": messages(session.Group()), "dms": dms,
				"offers": session.Group().FileOffers(), "selfOffer": session.Group().SelfFileOffer(), "rescinded": rescinded, "errors": errors})
		case "leave":
			if session != nil {
				session.Leave()
				session = nil
			}
			reply(map[string]any{"cmd": c.Cmd})
		case "quit":
			if session != nil {
				session.Leave()
			}
			reply(map[string]any{"cmd": c.Cmd})
			return
		default:
			fail(fmt.Errorf("unknown command"))
		}
	}
}

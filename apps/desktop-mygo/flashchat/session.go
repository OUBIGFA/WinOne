package flashchat

import (
	"errors"
	"strings"
	"sync"

	"winone/desktopmygo/flashchat/trystero"
)

// AppID is the Trystero app id of WinOne's flash chat; peers of another
// app id never meet.
const AppID = "winone-interconnect-chitchatter"

// relayRedundancy is how many default trackers the flash chat uses.
const relayRedundancy = 4

// DefaultNickname is the name of a peer that chose none.
const DefaultNickname = "WinOne"

// JoinOptions describe how to join or create a room, as the flash chat's
// lobby collects them.
type JoinOptions struct {
	// RoomID and Password join an existing room; both empty create one
	// with random ones.
	RoomID, Password string
	Nickname         string
	// Trackers replace the default ones; each must pass ValidRelayURL.
	Trackers []string
	RTC      *trystero.RTCConfig
	Files    FileTransfer
	// OnEvent hears every event of every room of the session, from the
	// rooms' goroutines.
	OnEvent func(room *ChatRoom, e Event)
	Logf    func(format string, args ...any)
	// Transport replaces the Trystero transport, for tests.
	Transport Transport
	// ReceiveDir is where files members send directly are saved.
	ReceiveDir func() (string, error)
}

// ErrRoomRequired is returned when only one of the room id and password
// was given.
var ErrRoomRequired = errors.New("room id and password required")

// Session is a joined room with its conversations: the group room and a
// direct room per peer, opened as soon as the peer joins so that private
// messages arrive before anyone opens the conversation (React's
// PeerChat.join and ensureDirect).
type Session struct {
	RoomID, Password, UserID string

	transport Transport
	registry  *Registry
	group     *ChatRoom
	base      Options
	onEvent   func(*ChatRoom, Event)

	files *files

	mu       sync.Mutex
	nickname string
	offs     []func()
	left     bool
}

// Join creates or joins a room.
func Join(o JoinOptions) (*Session, error) {
	id, secret := strings.TrimSpace(o.RoomID), o.Password
	if id == "" && secret == "" {
		id, secret = NewUUID(), NewUUID()
	}
	if id == "" || secret == "" {
		return nil, ErrRoomRequired
	}
	for _, u := range o.Trackers {
		if !trystero.ValidRelayURL(u) {
			return nil, &TrackerError{URL: u}
		}
	}
	nickname := strings.TrimSpace(o.Nickname)
	if nickname == "" {
		nickname = DefaultNickname
	}
	identity, err := NewIdentity()
	if err != nil {
		return nil, err
	}
	transport := o.Transport
	if transport == nil {
		t, err := NewTrysteroTransport(trystero.Config{AppID: AppID, Password: secret, RelayURLs: o.Trackers, Redundancy: relayRedundancy,
			RTC: o.RTC, Logf: o.Logf}, id)
		if err != nil {
			return nil, err
		}
		transport = t
	}
	s := &Session{RoomID: id, Password: secret, UserID: NewUUID(), transport: transport, registry: NewRegistry(), onEvent: o.OnEvent, nickname: nickname}
	s.base = Options{RoomID: id, UserID: s.UserID, CustomUsername: nickname, Identity: identity, Transport: transport, Files: o.Files,
		Password: secret, ShareHistoryOnJoin: true, Logf: o.Logf}
	s.group = NewChatRoom(s.base)
	s.registry.SetGroup(s.group)
	s.subscribe(s.group)
	s.mu.Lock()
	s.offs = append(s.offs, s.group.On(PeerJoin, func(e Event) { s.Direct(e.PeerID) }))
	s.mu.Unlock()
	dir := o.ReceiveDir
	if dir == nil {
		dir = func() (string, error) { return "", errors.New("no folder for received files") }
	}
	s.files = newFiles(s, dir, func(direct bool, peerID string) {
		if s.onEvent == nil {
			return
		}
		room := s.group
		if direct {
			if existing := s.Existing(peerID); existing != nil {
				room = existing
			}
		}
		s.onEvent(room, Event{Kind: DirectFileChange, PeerID: peerID})
	})
	transport.OnPeerLeave("WINONE_FILES", s.files.peerLeft)
	s.group.Join()
	return s, nil
}

// SendFile sends a file directly to everyone in the group ("") or to one
// member, over the room's own connections.
func (s *Session) SendFile(path, peerID string) error { return s.files.send(path, peerID) }

// Transfers lists the files sent and received directly in the group
// conversation ("") or the conversation with one member.
func (s *Session) Transfers(peerID string) []DirectFile { return s.files.snapshot(peerID) }

// TrackerError names a tracker address the flash chat does not accept.
type TrackerError struct{ URL string }

func (e *TrackerError) Error() string { return "invalid tracker: " + e.URL }

func (s *Session) subscribe(room *ChatRoom) {
	if s.onEvent == nil {
		return
	}
	var offs []func()
	for kind := MessageLogChange; kind <= Error; kind++ {
		offs = append(offs, room.On(kind, func(e Event) { s.onEvent(room, e) }))
	}
	s.mu.Lock()
	s.offs = append(s.offs, offs...)
	s.mu.Unlock()
}

// Group returns the group room.
func (s *Session) Group() *ChatRoom { return s.group }

// Nickname is this side's name in the room.
func (s *Session) Nickname() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nickname
}

// Direct returns the conversation with a peer, opening it when needed.
func (s *Session) Direct(peerID string) *ChatRoom {
	if peerID == "" {
		return s.group
	}
	s.mu.Lock()
	if s.left {
		s.mu.Unlock()
		return nil
	}
	if existing := s.registry.Direct(peerID); existing != nil {
		name := s.nickname
		s.mu.Unlock()
		existing.SetCustomUsername(name)
		return existing
	}
	o := s.base
	o.CustomUsername, o.TargetPeerID = s.nickname, peerID
	room := NewChatRoom(o)
	s.registry.SetDirect(peerID, room)
	s.mu.Unlock()
	s.subscribe(room)
	room.Join()
	return room
}

// Existing returns the conversation with a peer without opening one.
func (s *Session) Existing(peerID string) *ChatRoom {
	if peerID == "" {
		return s.group
	}
	return s.registry.Direct(peerID)
}

// SetNickname renames this side in the group room and every open
// conversation; "" uses DefaultNickname.
func (s *Session) SetNickname(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultNickname
	}
	s.mu.Lock()
	s.nickname = name
	s.base.CustomUsername = name
	s.mu.Unlock()
	s.group.SetCustomUsername(name)
	for _, id := range s.transport.Peers() {
		if room := s.registry.Direct(id); room != nil {
			room.SetCustomUsername(name)
		}
	}
}

// Leave leaves every room and the transport. The group room stops the
// files this side offered.
func (s *Session) Leave() {
	s.mu.Lock()
	if s.left {
		s.mu.Unlock()
		return
	}
	s.left = true
	offs := s.offs
	s.offs = nil
	s.mu.Unlock()
	for _, off := range offs {
		off()
	}
	s.files.stop()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.group.Leave() }()
	go func() { defer wg.Done(); s.registry.ClearDirect() }()
	wg.Wait()
	s.transport.Leave()
	s.registry.SetGroup(nil)
}

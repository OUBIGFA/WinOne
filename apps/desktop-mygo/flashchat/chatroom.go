package flashchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// EventKind is a ChatRoomEvent.
type EventKind int

const (
	// MessageLogChange: the transcript changed. Render from it.
	MessageLogChange EventKind = iota
	// MessageReceived: a message arrived from a peer.
	MessageReceived
	PeerListChange
	PeerJoin
	PeerLeave
	// PeerRename: a known peer changed its advertised username.
	PeerRename
	PeerVerification
	TypingStatusChange
	FileOfferChange
	// SelfFileOfferChange: what this client offers changed.
	SelfFileOfferChange
	SendingStateChange
	Error
)

// ErrorContext names the operation an Error event came out of.
type ErrorContext string

const (
	ContextAnnounceSelf     ErrorContext = "announceSelf"
	ContextGreetPeer        ErrorContext = "greetPeer"
	ContextLeave            ErrorContext = "leave"
	ContextRescindFileOffer ErrorContext = "rescindFileOffer"
	ContextSendInlineMedia  ErrorContext = "sendInlineMedia"
	ContextSendMessage      ErrorContext = "sendMessage"
	ContextSendTypingStatus ErrorContext = "sendTypingStatus"
	ContextStopSeeding      ErrorContext = "stopSeeding"
)

// Event is what a ChatRoom reports; the fields its kind uses are set.
type Event struct {
	Kind         EventKind
	PeerID       string
	Message      Message
	Peer         *Peer
	Verification VerificationState
	IsTyping     bool
	Offer        *FileOfferMetadata
	MagnetURI    string
	PreviousName string
	DisplayName  string
	Sending      bool
	Err          error
	Context      ErrorContext
}

// Options configure a ChatRoom (ChatRoomOptions).
type Options struct {
	RoomID         string
	UserID         string
	CustomUsername string
	Identity       *Identity
	Transport      Transport
	Files          FileTransfer
	// Password makes the room private: history is then withheld from
	// newcomers unless ShareHistoryOnJoin.
	Password           string
	Private            bool
	ShareHistoryOnJoin bool
	// TargetPeerID makes a direct-message room scoped to one peer.
	TargetPeerID string
	// TranscriptSizeLimit defaults to MessageTranscriptSizeLimit.
	TranscriptSizeLimit int
	// Now returns milliseconds since the epoch; NewID a unique id. Tests
	// set them; nil uses the clock and random UUIDs.
	Now   func() float64
	NewID func() string
	// Logf receives the core's warnings and errors.
	Logf func(format string, args ...any)
	// TypingExpiry defaults to TypingStatusExpiry milliseconds.
	TypingExpiry time.Duration
}

// ChatRoom is a headless Chitchatter chat room: the transcript, the peer
// list, presence and typing, spoken over a Transport. Methods are safe
// from any goroutine; those that send block until the transport took the
// data. Listeners run outside the room's lock, in the order of events.
type ChatRoom struct {
	o          Options
	direct     bool
	namespace  string
	limit      int
	mu         sync.Mutex
	pending    []Event
	listenerMu sync.Mutex
	listeners  map[EventKind]map[int]func(Event)
	nextID     int

	// Guarded by mu.
	customUsername string
	log            []Message
	peers          []Peer
	offers         map[string]FileOfferMetadata
	sending        bool
	typing         bool
	typingTimer    *time.Timer
	selfOffer      string
	selfFiles      []FileHandle
	joined, left   bool
	departed       map[string]bool
	senders        map[PeerAction]ActionHandle
}

// NewChatRoom prepares a room; Join binds it to the transport.
func NewChatRoom(o Options) *ChatRoom {
	if o.TranscriptSizeLimit == 0 {
		o.TranscriptSizeLimit = MessageTranscriptSizeLimit
	}
	if o.Now == nil {
		o.Now = func() float64 { return float64(time.Now().UnixMilli()) }
	}
	if o.NewID == nil {
		o.NewID = NewUUID
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if o.TypingExpiry == 0 {
		o.TypingExpiry = TypingStatusExpiry * time.Millisecond
	}
	r := &ChatRoom{o: o, direct: o.TargetPeerID != "", limit: o.TranscriptSizeLimit, customUsername: o.CustomUsername,
		offers: map[string]FileOfferMetadata{}, departed: map[string]bool{}, listeners: map[EventKind]map[int]func(Event){}}
	r.namespace = NamespaceGroup
	if r.direct {
		r.namespace = NamespaceDirect
	}
	return r
}

// RoomID, UserID and TargetPeerID are the room's identity.
func (r *ChatRoom) RoomID() string       { return r.o.RoomID }
func (r *ChatRoom) UserID() string       { return r.o.UserID }
func (r *ChatRoom) TargetPeerID() string { return r.o.TargetPeerID }

// IsDirect reports a direct-message room.
func (r *ChatRoom) IsDirect() bool { return r.direct }

func (r *ChatRoom) private() bool { return r.o.Private || r.o.Password != "" }

// ------------------------------------------------------------------ events

// On subscribes to an event; the returned function unsubscribes.
func (r *ChatRoom) On(kind EventKind, fn func(Event)) func() {
	r.listenerMu.Lock()
	r.nextID++
	id := r.nextID
	if r.listeners[kind] == nil {
		r.listeners[kind] = map[int]func(Event){}
	}
	r.listeners[kind][id] = fn
	r.listenerMu.Unlock()
	return func() {
		r.listenerMu.Lock()
		delete(r.listeners[kind], id)
		r.listenerMu.Unlock()
	}
}

func (r *ChatRoom) emit(e Event) { r.pending = append(r.pending, e) }

// unlock releases the room, then runs the listeners of what changed: a
// listener may call the room back.
func (r *ChatRoom) unlock() {
	events := r.pending
	r.pending = nil
	r.mu.Unlock()
	for _, e := range events {
		r.listenerMu.Lock()
		var fns []func(Event)
		for _, fn := range r.listeners[e.Kind] {
			fns = append(fns, fn)
		}
		r.listenerMu.Unlock()
		for _, fn := range fns {
			fn(e)
		}
	}
}

// ---------------------------------------------------------------- lifecycle

// Join binds the protocol to the transport and announces this peer.
func (r *ChatRoom) Join() {
	r.mu.Lock()
	if r.joined {
		r.unlock()
		return
	}
	r.joined = true
	r.senders = map[PeerAction]ActionHandle{}
	receivers := map[PeerAction]func(json.RawMessage, string){
		ActionMessage:            r.handleMessage,
		ActionMediaMessage:       r.handleInlineMedia,
		ActionMessageTranscript:  r.handleTranscript,
		ActionPeerMetadata:       r.handleMetadata,
		ActionTypingStatusChange: r.handleTypingStatus,
		ActionFileOffer:          r.handleFileOffer,
	}
	for action, receive := range receivers {
		h := r.o.Transport.MakeAction(ActionName(r.namespace, action))
		h.Connect(receive)
		r.senders[action] = h
	}
	r.unlock()
	if !r.direct {
		r.o.Transport.OnPeerJoin(HookNewPeer, r.handlePeerJoin)
		r.o.Transport.OnPeerLeave(HookNewPeer, r.handlePeerLeave)
	}
	r.announceSelf("")
}

// Leave detaches the room's receivers. It does not leave the transport. A
// group room also stops seeding: its offers are reachable only while it is
// in the room.
func (r *ChatRoom) Leave() {
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	r.left = true
	r.clearTypingExpiry()
	wasTyping := r.typing
	senders := r.senders
	r.unlock()
	if wasTyping {
		r.sendTypingStatus(false, "", senders)
	}
	r.mu.Lock()
	for _, h := range senders {
		h.Disconnect()
	}
	r.senders = nil
	r.unlock()
	if !r.direct && r.o.Files != nil {
		if err := r.o.Files.RescindAll(); err != nil {
			r.reportError(err, ContextLeave)
		}
	}
}

// ------------------------------------------------------------- state access

// MessageLog returns the transcript.
func (r *ChatRoom) MessageLog() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Message(nil), r.log...)
}

// Peers returns the peers that announced themselves.
func (r *ChatRoom) Peers() []Peer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Peer(nil), r.peers...)
}

// CustomUsername is this side's chosen name.
func (r *ChatRoom) CustomUsername() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.customUsername
}

// IsTyping reports this side's typing state.
func (r *ChatRoom) IsTyping() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.typing
}

// IsMessageSending reports a send in progress.
func (r *ChatRoom) IsMessageSending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sending
}

// FileOffers returns who offers what, by peer id.
func (r *ChatRoom) FileOffers() map[string]FileOfferMetadata {
	r.mu.Lock()
	defer r.mu.Unlock()
	offers := make(map[string]FileOfferMetadata, len(r.offers))
	for k, v := range r.offers {
		offers[k] = v
	}
	return offers
}

// SelfFileOffer is the address of this side's current offer, "" when none.
func (r *ChatRoom) SelfFileOffer() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selfOffer
}

func (r *ChatRoom) setSelfFileOffer(magnetURI string, files []FileHandle) {
	changed := r.selfOffer != magnetURI
	r.selfOffer, r.selfFiles = magnetURI, files
	if changed {
		r.emit(Event{Kind: SelfFileOfferChange, MagnetURI: magnetURI})
	}
}

// ------------------------------------------------------------------ commands

// SendMessage posts text: recorded at once, then sent, then marked
// delivered. A failed send is reported as an Error event and returned.
func (r *ChatRoom) SendMessage(text string) error {
	r.mu.Lock()
	if r.sending || r.left {
		r.unlock()
		return ErrNotSent
	}
	message := Message{AuthorID: r.o.UserID, Text: text, TimeSent: r.o.Now(), ID: r.o.NewID()}
	r.stopTyping()
	r.setSending(true)
	r.appendToLog(message)
	sender := r.senders[ActionMessage]
	r.unlock()
	err := r.send(sender, message, "")
	r.mu.Lock()
	if err == nil {
		r.markDelivered(message.ID)
	}
	r.setSending(false)
	r.unlock()
	if err != nil {
		r.reportError(err, ContextSendMessage)
	}
	return err
}

// ErrNotSent is SendMessage's answer while another send runs or after the
// room was left: nothing was recorded or sent.
var ErrNotSent = errors.New("message not sent")

// unsent is a message's wire shape before it is received: no receipt.
func unsent(m Message) Message {
	m.Received, m.TimeReceived = false, 0
	return m
}

func (r *ChatRoom) send(h ActionHandle, payload any, target string) error {
	if h == nil {
		return nil
	}
	if m, ok := payload.(Message); ok {
		payload = unsent(m)
	}
	if target == "" {
		target = r.o.TargetPeerID
	}
	return h.Send(payload, target)
}

// SendInlineMedia offers files and posts them into the transcript.
func (r *ChatRoom) SendInlineMedia(files []FileHandle) error {
	r.mu.Lock()
	if r.left {
		r.unlock()
		return nil
	}
	r.setSending(true)
	r.unlock()
	err := func() error {
		magnetURI, err := r.o.Files.Offer(files, r.o.RoomID)
		if err != nil {
			return err
		}
		r.mu.Lock()
		media := Message{AuthorID: r.o.UserID, Media: true, MagnetURI: magnetURI, TimeSent: r.o.Now(), ID: r.o.NewID()}
		r.appendToLog(media)
		sender := r.senders[ActionMediaMessage]
		r.unlock()
		if err := r.send(sender, media, ""); err != nil {
			return err
		}
		r.mu.Lock()
		r.markDelivered(media.ID)
		r.unlock()
		return nil
	}()
	r.mu.Lock()
	r.setSending(false)
	r.unlock()
	if err != nil {
		r.reportError(err, ContextSendInlineMedia)
	}
	return err
}

// NotifyTyping reports a keystroke: the first announces typing, the rest
// push back its expiry.
func (r *ChatRoom) NotifyTyping() {
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	announce := !r.typing
	r.typing = true
	r.clearTypingExpiry()
	r.typingTimer = time.AfterFunc(r.o.TypingExpiry, func() {
		r.mu.Lock()
		r.stopTyping()
		r.unlock()
	})
	senders := r.senders
	r.unlock()
	if announce {
		go r.sendTypingStatus(true, "", senders)
	}
}

// SetCustomUsername changes this side's name and announces it.
func (r *ChatRoom) SetCustomUsername(name string) {
	r.mu.Lock()
	if r.customUsername == name {
		r.unlock()
		return
	}
	r.customUsername = name
	r.unlock()
	r.announceSelf("")
}

// OfferFiles offers files to the room and returns their address, or ""
// when the room was left. A failed announcement stops the share again.
func (r *ChatRoom) OfferFiles(files []FileHandle) (string, error) {
	r.mu.Lock()
	left := r.left
	r.unlock()
	if left {
		return "", nil
	}
	magnetURI, err := r.o.Files.Offer(files, r.o.RoomID)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.setSelfFileOffer(magnetURI, files)
	sender := r.senders[ActionFileOffer]
	r.unlock()
	if err := r.send(sender, FileOfferMetadata{MagnetURI: magnetURI, IsAllInlineMedia: IsEveryFileInlineMedia(files)}, ""); err != nil {
		r.mu.Lock()
		r.setSelfFileOffer("", nil)
		r.unlock()
		r.stopSeeding(magnetURI)
		return "", err
	}
	return magnetURI, nil
}

// RescindFileOffer stops offering: locally at once, then to the peers.
// Inline media stays seeded, as the transcript still shows it.
func (r *ChatRoom) RescindFileOffer() {
	r.mu.Lock()
	magnetURI := r.selfOffer
	if magnetURI == "" {
		r.unlock()
		return
	}
	allInline := IsEveryFileInlineMedia(r.selfFiles)
	r.setSelfFileOffer("", nil)
	sender := r.senders[ActionFileOffer]
	r.unlock()
	if err := r.send(sender, nil, ""); err != nil {
		r.reportError(err, ContextRescindFileOffer)
	}
	if allInline || !r.o.Files.IsOffering(magnetURI) {
		return
	}
	if err := r.o.Files.Rescind(magnetURI); err != nil {
		r.reportError(err, ContextRescindFileOffer)
	}
}

// ------------------------------------------------------------------ handlers

func (r *ChatRoom) fromRoomPeer(peerID string) bool { return !r.direct || peerID == r.o.TargetPeerID }

func (r *ChatRoom) handleMessage(data json.RawMessage, peerID string) {
	var m Message
	if !r.fromRoomPeer(peerID) || json.Unmarshal(data, &m) != nil || m.Media {
		return
	}
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	m.Received, m.TimeReceived = true, r.o.Now()
	r.appendToLog(m)
	r.updatePeer(peerID, func(p *Peer) { p.TypingGroup = false })
	r.emit(Event{Kind: MessageReceived, Message: m, PeerID: peerID})
	r.unlock()
}

func (r *ChatRoom) handleInlineMedia(data json.RawMessage, peerID string) {
	var m Message
	if !r.fromRoomPeer(peerID) || json.Unmarshal(data, &m) != nil || !m.Media {
		return
	}
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	m.Received, m.TimeReceived = true, r.o.Now()
	r.appendToLog(m)
	r.emit(Event{Kind: MessageReceived, Message: m, PeerID: peerID})
	r.unlock()
}

func (r *ChatRoom) handleTranscript(data json.RawMessage, peerID string) {
	if !r.fromRoomPeer(peerID) {
		return
	}
	var raw []json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	var transcript []Message
	for _, item := range raw {
		var m Message
		if json.Unmarshal(item, &m) == nil {
			transcript = append(transcript, m)
		}
	}
	r.mu.Lock()
	defer r.unlock()
	if r.left {
		return
	}
	// Newcomers may have sent before the greeting came: merge the history
	// without replacing local entries or repeating several peers' copies.
	// Direct conversations never publish their history.
	if r.o.ShareHistoryOnJoin {
		if r.direct {
			return
		}
		type key struct{ author, id string }
		order := []key{}
		entries := map[key]Message{}
		for _, m := range append(transcript, r.log...) {
			k := key{m.AuthorID, m.ID}
			if _, ok := entries[k]; !ok {
				order = append(order, k)
			}
			entries[k] = m
		}
		merged := make([]Message, 0, len(order))
		for _, k := range order {
			merged = append(merged, entries[k])
		}
		sort.SliceStable(merged, func(i, j int) bool {
			if merged[i].TimeSent != merged[j].TimeSent {
				return merged[i].TimeSent < merged[j].TimeSent
			}
			return merged[i].ID < merged[j].ID
		})
		r.setLog(merged)
		return
	}
	if len(r.log) > 0 {
		return
	}
	r.setLog(transcript)
}

func (r *ChatRoom) handleMetadata(data json.RawMessage, peerID string) {
	var md UserMetadata
	if !r.fromRoomPeer(peerID) || json.Unmarshal(data, &md) != nil {
		return
	}
	state, ok := VerifyIdentity(md.PublicKeyString, md.IdentitySignatureBase64, r.o.RoomID, md.UserID)
	if !ok {
		r.o.Logf("Could not parse the public key advertised by peer %s; ignoring its metadata", peerID)
		return
	}
	if state == Unverified {
		r.o.Logf("Peer verification failed, marking peer as unverified")
	}
	r.mu.Lock()
	// A departed peer's metadata still in flight must not bring it back.
	if r.left || r.departed[peerID] {
		r.unlock()
		return
	}
	var previous *Peer
	index := -1
	for i := range r.peers {
		if r.peers[i].PeerID == peerID {
			p := r.peers[i]
			previous, index = &p, i
		}
	}
	peers := append([]Peer(nil), r.peers...)
	added := index < 0
	renamed := false
	if added {
		peers = append(peers, Peer{PeerID: peerID, UserID: md.UserID, CustomUsername: md.CustomUsername, PublicKey: md.PublicKeyString, Verification: state})
	} else {
		renamed = previous.CustomUsername != md.CustomUsername || previous.UserID != md.UserID
		p := &peers[index]
		p.UserID, p.CustomUsername, p.PublicKey, p.Verification = md.UserID, md.CustomUsername, md.PublicKeyString, state
	}
	r.setPeers(peers)
	r.emit(Event{Kind: PeerVerification, PeerID: peerID, Verification: state})
	senders := r.senders
	if !added && renamed {
		r.emit(Event{Kind: PeerRename, PeerID: peerID, PreviousName: PeerDisplayName(*previous), DisplayName: r.displayUsername(md.UserID, peers)})
	}
	r.unlock()
	if added {
		// It may not know us: its room could have been listening after our
		// greeting on its join went out. Greet it now, and tell it whether
		// we are typing.
		go func() {
			r.greetPeer(peerID)
			r.mu.Lock()
			typing := r.typing
			r.unlock()
			r.sendTypingStatus(typing, peerID, senders)
		}()
	}
}

// displayUsername resolves a user id among the peers, as
// getDisplayUsername.
func (r *ChatRoom) displayUsername(userID string, peers []Peer) string {
	custom := ""
	if userID == r.o.UserID {
		custom = r.customUsername
	} else {
		for _, p := range peers {
			if p.UserID == userID {
				custom = p.CustomUsername
				break
			}
		}
	}
	return DisplayName(custom, userID)
}

func (r *ChatRoom) handleTypingStatus(data json.RawMessage, peerID string) {
	var status TypingStatus
	if !r.fromRoomPeer(peerID) || json.Unmarshal(data, &status) != nil {
		return
	}
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	r.updatePeer(peerID, func(p *Peer) {
		p.TypingGroup = status.IsTyping && !r.direct
		p.TypingDirect = status.IsTyping && r.direct
	})
	r.emit(Event{Kind: TypingStatusChange, PeerID: peerID, IsTyping: status.IsTyping})
	r.unlock()
}

func (r *ChatRoom) handleFileOffer(data json.RawMessage, peerID string) {
	if !r.fromRoomPeer(peerID) {
		return
	}
	var offer *FileOfferMetadata
	if strings.TrimSpace(string(data)) != "null" {
		var o FileOfferMetadata
		if json.Unmarshal(data, &o) != nil || o.MagnetURI == "" {
			return
		}
		offer = &o
	}
	r.mu.Lock()
	if r.left {
		r.unlock()
		return
	}
	var stop string
	if offer != nil {
		r.offers[peerID] = *offer
	} else {
		if withdrawn, ok := r.offers[peerID]; ok && r.o.Files != nil && shouldRescindOffer(&withdrawn, r.o.Files.IsOffering) {
			stop = withdrawn.MagnetURI
		}
		delete(r.offers, peerID)
	}
	offered := ""
	if offer != nil {
		offered = offer.MagnetURI
	}
	r.updatePeer(peerID, func(p *Peer) { p.OfferedFileID = offered })
	r.emit(Event{Kind: FileOfferChange, PeerID: peerID, Offer: offer})
	r.unlock()
	if stop != "" {
		r.stopSeeding(stop)
	}
}

func (r *ChatRoom) handlePeerJoin(peerID string) {
	r.mu.Lock()
	delete(r.departed, peerID)
	r.emit(Event{Kind: PeerJoin, PeerID: peerID})
	r.unlock()
	go r.greetPeer(peerID)
}

// greetPeer introduces this side to a peer and hands over what it is owed
// on arrival: identity, history (unless withheld) and the standing offer.
// It runs on a join and on metadata from a peer not yet known; either
// alone could miss a peer whose room was not listening yet.
func (r *ChatRoom) greetPeer(peerID string) {
	r.mu.Lock()
	withhold := (r.private() && !r.o.ShareHistoryOnJoin) || r.direct
	var history []Message
	for _, m := range r.log {
		if m.Received {
			history = append(history, m)
		}
	}
	offer := r.selfOffer
	allInline := IsEveryFileInlineMedia(r.selfFiles)
	senders := r.senders
	r.unlock()
	errs := make(chan error, 3)
	go func() { errs <- r.announceSelf(peerID) }()
	go func() {
		if withhold || senders == nil {
			errs <- nil
			return
		}
		if history == nil {
			history = []Message{}
		}
		errs <- senders[ActionMessageTranscript].Send(history, peerID)
	}()
	go func() {
		if offer == "" || senders == nil {
			errs <- nil
			return
		}
		errs <- senders[ActionFileOffer].Send(FileOfferMetadata{MagnetURI: offer, IsAllInlineMedia: allInline}, peerID)
	}()
	for range 3 {
		if err := <-errs; err != nil {
			r.reportError(err, ContextGreetPeer)
			return
		}
	}
}

func (r *ChatRoom) handlePeerLeave(peerID string) {
	r.mu.Lock()
	r.departed[peerID] = true
	var peer *Peer
	for i := range r.peers {
		if r.peers[i].PeerID == peerID {
			p := r.peers[i]
			peer = &p
		}
	}
	var stop string
	if offer, ok := r.offers[peerID]; ok {
		if r.o.Files != nil && shouldRescindOffer(&offer, r.o.Files.IsOffering) {
			stop = offer.MagnetURI
		}
		delete(r.offers, peerID)
		r.emit(Event{Kind: FileOfferChange, PeerID: peerID})
	}
	if peer != nil {
		var rest []Peer
		for _, p := range r.peers {
			if p.PeerID != peerID {
				rest = append(rest, p)
			}
		}
		r.setPeers(rest)
	}
	r.emit(Event{Kind: PeerLeave, PeerID: peerID, Peer: peer})
	r.unlock()
	if stop != "" {
		r.stopSeeding(stop)
	}
}

// NotifyPeerLeft tells a direct-message room its correspondent left; the
// group room holds the transport's only leave slot (ChatRoomRegistry
// forwards it).
func (r *ChatRoom) NotifyPeerLeft(peerID string) { r.handlePeerLeave(peerID) }

// NotifyPeerJoined is NotifyPeerLeft's mirror, so a correspondent whose
// connection blipped is taken back.
func (r *ChatRoom) NotifyPeerJoined(peerID string) { r.handlePeerJoin(peerID) }

// ------------------------------------------------------------------ internals

func (r *ChatRoom) announceSelf(target string) error {
	r.mu.Lock()
	sender := r.senders[ActionPeerMetadata]
	name := r.customUsername
	r.unlock()
	if sender == nil {
		return nil
	}
	err := func() error {
		proof, err := r.o.Identity.Sign(r.o.RoomID, r.o.UserID)
		if err != nil {
			return err
		}
		return r.send(sender, UserMetadata{UserID: r.o.UserID, CustomUsername: name, PublicKeyString: r.o.Identity.PublicKey(), IdentitySignatureBase64: proof}, target)
	}()
	if err != nil {
		r.reportError(err, ContextAnnounceSelf)
	}
	return nil
}

func (r *ChatRoom) sendTypingStatus(typing bool, target string, senders map[PeerAction]ActionHandle) {
	if senders == nil {
		return
	}
	if err := r.send(senders[ActionTypingStatusChange], TypingStatus{IsTyping: typing}, target); err != nil {
		r.reportError(err, ContextSendTypingStatus)
	}
}

// stopTyping runs with mu held.
func (r *ChatRoom) stopTyping() {
	r.clearTypingExpiry()
	if !r.typing {
		return
	}
	r.typing = false
	senders := r.senders
	go r.sendTypingStatus(false, "", senders)
}

func (r *ChatRoom) clearTypingExpiry() {
	if r.typingTimer != nil {
		r.typingTimer.Stop()
		r.typingTimer = nil
	}
}

// updatePeer applies host-owned state to a peer, and reports a change
// only when a value changed.
func (r *ChatRoom) updatePeer(peerID string, change func(*Peer)) {
	for i := range r.peers {
		if r.peers[i].PeerID != peerID {
			continue
		}
		next := r.peers[i]
		change(&next)
		if next == r.peers[i] {
			return
		}
		peers := append([]Peer(nil), r.peers...)
		peers[i] = next
		r.setPeers(peers)
		return
	}
}

func (r *ChatRoom) appendToLog(m Message) {
	r.setLog(append(append([]Message(nil), r.log...), m))
}

// markDelivered stamps an own message delivered once its send resolved.
func (r *ChatRoom) markDelivered(id string) {
	log := append([]Message(nil), r.log...)
	for i := range log {
		if log[i].ID == id && !log[i].Received {
			log[i].Received, log[i].TimeReceived = true, r.o.Now()
		}
	}
	r.setLog(log)
}

func (r *ChatRoom) setLog(messages []Message) {
	kept, evicted := applyTranscriptLimit(messages, r.limit)
	// Inline media scrolled out of the transcript is unreachable: stop it.
	for _, m := range evicted {
		if m.Media && r.o.Files != nil && r.o.Files.IsOffering(m.MagnetURI) {
			go r.stopSeeding(m.MagnetURI)
		}
	}
	r.log = kept
	r.emit(Event{Kind: MessageLogChange})
}

func (r *ChatRoom) setPeers(peers []Peer) {
	r.peers = peers
	r.emit(Event{Kind: PeerListChange})
}

func (r *ChatRoom) setSending(sending bool) {
	if r.sending == sending {
		return
	}
	r.sending = sending
	r.emit(Event{Kind: SendingStateChange, Sending: sending})
}

func (r *ChatRoom) stopSeeding(magnetURI string) {
	if r.o.Files == nil {
		return
	}
	if err := r.o.Files.Rescind(magnetURI); err != nil {
		r.reportError(err, ContextStopSeeding)
	}
}

func (r *ChatRoom) reportError(err error, context ErrorContext) {
	if err == nil {
		err = errors.New("unknown error")
	}
	r.o.Logf("ChatRoom.%s %v", context, err)
	r.mu.Lock()
	r.emit(Event{Kind: Error, Err: err, Context: context})
	r.unlock()
}

// String names the room for logs.
func (r *ChatRoom) String() string {
	if r.direct {
		return fmt.Sprintf("dm(%s)", r.o.TargetPeerID)
	}
	return "group"
}

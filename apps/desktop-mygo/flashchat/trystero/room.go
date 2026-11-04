package trystero

import (
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
)

// Timings of strategy.ts, signal-handler.ts, handshake.ts and
// data-limits.ts. Peers keep each other's pace with them.
const (
	offerPostAnswerTTL      = 23333 * time.Millisecond
	answeringTTL            = 23333 * time.Millisecond
	disconnectedPeerGrace   = 7533 * time.Millisecond
	handshakeTimeout        = 10 * time.Second
	transferTimeout         = 120 * time.Second
	signalDedupeWindow      = 4 * time.Second
	leaveDelay              = 99 * time.Millisecond
	answerReplayMinInterval = time.Second
)

// Message is what an action received: its payload in its format, from a
// peer.
type Message struct {
	PeerID string
	Format byte
	Data   []byte
}

// Room is a joined Trystero room on the WebTorrent strategy. Its state
// lives on one queue; its handlers run, in order, on another.
type Room struct {
	cfg             Config
	appID, roomID   string
	selfID          string
	rootTopic, hash string
	token           string
	aead            cipher.AEAD
	api             *webrtc.API
	rtc             webrtc.Configuration
	pattern         candidatePattern
	loop, out       *queue
	logf            func(string, ...any)

	// Room queue only.
	relays     []*relay
	offers     map[string]*pooledOffer
	generating bool
	waiters    []func(bool)
	signals    map[string]time.Time
	states     map[string]*peerState
	members    map[string]*member
	actions    map[string]*action
	outgoing   map[string]map[uint32]*outgoing
	nextID     uint32
	reserved   int
	onJoin     func(string)
	onLeave    func(string)
	left       bool
}

// Join joins roomID and starts looking for its peers on the trackers.
func Join(cfg Config, roomID string) (*Room, error) {
	if cfg.AppID == "" {
		return nil, errors.New("config is missing appId")
	}
	if roomID == "" {
		return nil, errors.New("roomId required")
	}
	aead, err := signalCipher(cfg.Password, cfg.AppID, roomID)
	if err != nil {
		return nil, err
	}
	root := sha1Topic(topicPath(libName, cfg.AppID, roomID))
	r := &Room{cfg: cfg, appID: cfg.AppID, roomID: roomID, selfID: genID(20), rootTopic: root, hash: infoHash(root),
		token: roomNamespace(cfg.AppID, roomID), aead: aead, loop: newQueue(), out: newQueue(),
		offers: map[string]*pooledOffer{}, signals: map[string]time.Time{}, states: map[string]*peerState{},
		members: map[string]*member{}, actions: map[string]*action{}, outgoing: map[string]map[uint32]*outgoing{}}
	r.logf = cfg.Logf
	if r.logf == nil {
		r.logf = func(string, ...any) {}
	}
	servers := cfg.RTC.iceServers()
	for _, s := range servers {
		r.rtc.ICEServers = append(r.rtc.ICEServers, webrtc.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential})
	}
	if cfg.RTC != nil && cfg.RTC.ICETransportPolicy == "relay" {
		r.rtc.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	}
	r.pattern = patternFor(servers)
	var se webrtc.SettingEngine
	se.LoggerFactory = &logging.DefaultLoggerFactory{Writer: io.Discard}
	// Peers on other networks must reach this computer even behind a
	// proxy's TUN adapter (directNet).
	if dn, err := newDirectNet(); err == nil {
		se.SetNet(dn)
	}
	r.api = webrtc.NewAPI(webrtc.WithSettingEngine(se))
	r.registerInternal()
	for _, u := range cfg.relays() {
		rl := &relay{room: r, url: u, announceMs: defaultAnnounce, quit: make(chan struct{})}
		r.relays = append(r.relays, rl)
		go rl.run()
	}
	return r, nil
}

// SelfID is this peer's id in the room, as Trystero's selfId.
func (r *Room) SelfID() string { return r.selfID }

// SetHandlers sets who hears peers joining and leaving; join hears the
// peers already in the room at once, as Trystero's onPeerJoin setter.
func (r *Room) SetHandlers(join, leave func(peerID string)) {
	r.loop.post(func() {
		r.onJoin, r.onLeave = join, leave
		if join == nil {
			return
		}
		for id, m := range r.members {
			if m.active {
				r.out.post(func() { join(id) })
			}
		}
	})
}

// Peers returns the peers that completed the handshake.
func (r *Room) Peers() []string {
	return call(r.loop, func() []string {
		var ids []string
		for id, m := range r.members {
			if m.active {
				ids = append(ids, id)
			}
		}
		return ids
	})
}

// MakeAction registers handler for an action's messages. Messages that
// came for it before are delivered now, as Trystero queues them until an
// action is made; a second registration replaces the handler.
func (r *Room) MakeAction(typ string, handler func(Message)) {
	r.loop.post(func() {
		if r.left {
			return
		}
		a := r.actions[typ]
		if a == nil || a.internal != nil {
			a = &action{typ: typ}
			r.actions[typ] = a
		}
		a.public = handler
		for _, m := range r.members {
			r.flushQueued(m, typ)
		}
	})
}

// Leave tells the peers this side leaves, closes every connection and
// stops looking for peers. It blocks for about 100 ms, as Trystero gives
// the goodbye a moment to go out.
func (r *Room) Leave() {
	members := call(r.loop, func() []*member {
		if r.left {
			return nil
		}
		r.left = true
		var list []*member
		for _, m := range r.members {
			_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_leave", FormatString, nil)))
			_ = m.p.send(presenceFrame(r.token, false))
			list = append(list, m)
		}
		for _, rl := range r.relays {
			rl.stop()
		}
		for id, o := range r.offers {
			o.peer.destroy()
			delete(r.offers, id)
		}
		return list
	})
	if members != nil {
		time.Sleep(leaveDelay)
	}
	r.loop.post(func() {
		for _, m := range r.members {
			r.exitPeer(m, false)
		}
		for _, st := range r.states {
			for _, p := range []*peer{st.answering, st.offer, st.connected} {
				if p != nil {
					p.destroy()
				}
			}
			st.stopTimers()
		}
		r.states = map[string]*peerState{}
	})
	r.loop.stop()
	<-r.loop.done
	r.out.stop()
}

// ---------------------------------------------------------------- signalling

// peerState is signal-handler.ts's state of one remote peer: the
// connection in use, an answer being made to its offer, and this side's
// offer it answered.
type peerState struct {
	connected      *peer
	unhealthySince time.Time

	answering     *peer
	answerSent    bool
	replay        *answerReplay
	answerTimer   *time.Timer
	offer         *peer
	offerID       string
	offerAnswered bool
	offerTimer    *time.Timer
	reported      bool
}

type answerReplay struct {
	offer, offerID string
	message        []byte
	lastSent       map[*relay]time.Time
	relays         map[*relay]bool
}

func (st *peerState) stopTimers() {
	if st.answerTimer != nil {
		st.answerTimer.Stop()
		st.answerTimer = nil
	}
	if st.offerTimer != nil {
		st.offerTimer.Stop()
		st.offerTimer = nil
	}
}

func (r *Room) state(id string) *peerState {
	st := r.states[id]
	if st == nil {
		st = &peerState{}
		r.states[id] = st
	}
	return st
}

func (r *Room) resetAnswering(st *peerState) {
	if st.answerTimer != nil {
		st.answerTimer.Stop()
		st.answerTimer = nil
	}
	st.answering, st.answerSent, st.replay = nil, false, nil
}

func (r *Room) clearAnswering(st *peerState, p *peer) {
	if st.answering != p {
		return
	}
	r.resetAnswering(st)
	if st.connected != p {
		p.destroy()
	}
}

func (r *Room) resetOffer(st *peerState) {
	if st.offerTimer != nil {
		st.offerTimer.Stop()
		st.offerTimer = nil
	}
	if st.offer != nil && st.offer != st.connected {
		st.offer.destroy()
	}
	st.offer, st.offerID, st.offerAnswered, st.reported = nil, "", false, false
}

// reportFailure is the "could not connect after exchanging SDP" join
// error; Chitchatter does not listen for join errors, so it is a warning.
func (r *Room) reportFailure(st *peerState, id string) {
	if r.left || st.connected != nil || st.reported {
		return
	}
	st.reported = true
	hint := "configure TURN servers with turnConfig or rtcConfig.iceServers"
	if r.pattern == relayCandidate {
		hint = "check that your TURN server URLs and credentials are reachable by both peers"
	}
	r.logf("Trystero: could not connect to peer %s after exchanging SDP; %s", id, hint)
}

// signal is the start of createSignalHandler: it drops signals from peers
// already connected, and lets those whose connection went stale through.
func (r *Room) signal(id string) bool {
	if r.left || id == "" || id == r.selfID {
		return false
	}
	st := r.states[id]
	if st != nil && st.connected != nil {
		switch st.connected.health() {
		case "live":
			st.unhealthySince = time.Time{}
			return false
		case "stale":
			r.clearConnected(st, id)
		default:
			now := time.Now()
			if st.unhealthySince.IsZero() {
				st.unhealthySince = now
			}
			if now.Sub(st.unhealthySince) < disconnectedPeerGrace {
				return false
			}
			r.clearConnected(st, id)
		}
	}
	if m := r.members[id]; m != nil && m.p.health() != "stale" {
		return false
	}
	return true
}

func (r *Room) clearConnected(st *peerState, id string) {
	p := st.connected
	st.connected, st.unhealthySince = nil, time.Time{}
	if m := r.members[id]; m != nil && m.p == p {
		r.exitPeer(m, true)
	} else if p != nil {
		p.destroy()
	}
}

// handleOffer answers a peer's offer the tracker passed on.
func (r *Room) handleOffer(rl *relay, id, offer, offerID string) {
	st := r.state(id)
	if st.answering != nil || st.offerAnswered {
		replay := st.replay
		if st.answering != nil && !st.answering.dead.Load() && replay != nil && replay.offer == offer && replay.offerID == offerID &&
			time.Since(replay.lastSent[rl]) >= answerReplayMinInterval {
			replay.lastSent[rl] = time.Now()
			replay.relays[rl] = true
			if replay.message != nil {
				rl.send(replay.message)
			}
		}
		return
	}
	if st.offer != nil {
		if r.selfID < id {
			return
		}
		r.resetOffer(st)
	}
	plain, err := decryptSignal(r.aead, offer)
	if err != nil {
		r.logf("Trystero: incorrect room password when decrypting offer from %s", id)
		return
	}
	p, err := r.newPeer(false)
	if err != nil {
		r.logf("Trystero: %v", err)
		return
	}
	replay := &answerReplay{offer: offer, offerID: offerID, lastSent: map[*relay]time.Time{rl: time.Now()}, relays: map[*relay]bool{rl: true}}
	st.answering, st.answerSent, st.replay, st.reported = p, false, replay, false
	st.answerTimer = time.AfterFunc(answeringTTL, func() {
		r.loop.post(func() {
			if st.connected != nil || st.answering != p {
				return
			}
			if st.answerSent {
				r.reportFailure(st, id)
			}
			r.clearAnswering(st, p)
		})
	})
	p.onConnect = func() { r.connectPeer(p, id) }
	p.onClose = func() {
		if st.answering == p && st.connected == nil && st.answerSent {
			r.reportFailure(st, id)
		}
		r.clearAnswering(st, p)
		r.disconnectPeer(p, id)
	}
	go func() {
		answer, err := p.answer(plain)
		r.loop.post(func() {
			if r.left || st.answering != p || p.dead.Load() {
				return
			}
			if err != nil {
				p.destroy()
				return
			}
			sealed, err := encryptSignal(r.aead, answer)
			if err != nil {
				p.destroy()
				return
			}
			message, _ := marshalJSON(map[string]any{"action": "announce", "info_hash": r.hash, "peer_id": r.selfID,
				"answer": map[string]string{"type": "answer", "sdp": sealed}, "offer_id": offerID, "to_peer_id": id})
			st.answerSent, replay.message = true, message
			for target := range replay.relays {
				replay.lastSent[target] = time.Now()
				target.send(message)
			}
		})
	}()
}

// handleAnswer applies a peer's answer to this side's pooled offer.
func (r *Room) handleAnswer(id, answer, offerID string, p *peer) {
	plain, err := decryptSignal(r.aead, answer)
	if err != nil {
		p.destroy()
		r.logf("Trystero: incorrect room password when decrypting answer from %s", id)
		return
	}
	st := r.state(id)
	if st.connected != nil || st.offerAnswered || (st.answering != nil && !st.answering.dead.Load() && r.selfID > id) {
		p.destroy()
		return
	}
	if st.answering != nil {
		r.clearAnswering(st, st.answering)
	}
	r.resetOffer(st)
	st.offer, st.offerID, st.offerAnswered = p, offerID, true
	st.offerTimer = time.AfterFunc(offerPostAnswerTTL, func() {
		r.loop.post(func() {
			if st.connected != nil || st.offer != p {
				return
			}
			r.reportFailure(st, id)
			r.resetOffer(st)
		})
	})
	p.onConnect = func() { r.connectPeer(p, id) }
	p.onClose = func() {
		if st.offer == p && st.connected == nil {
			r.reportFailure(st, id)
			r.resetOffer(st)
		}
		r.disconnectPeer(p, id)
	}
	go func() {
		if err := p.accept(plain); err != nil {
			r.loop.post(p.destroy)
		}
	}()
}

func (r *Room) disconnectPeer(p *peer, id string) {
	if r.left {
		return
	}
	if st := r.states[id]; st != nil && st.connected == p {
		st.connected, st.unhealthySince = nil, time.Time{}
	}
}

// --------------------------------------------------------------- membership

// member is a connected peer of the room: Trystero's peerMap entry, with
// its handshake and transfers.
type member struct {
	id     string
	p      *peer
	active bool
	failed bool

	initiator   bool
	challenge   string
	passed      bool
	remoteReady bool
	hsTimer     *time.Timer

	incoming  map[uint32]*incoming
	receiving *incoming
	waiting   []*incoming
	queued    []*queuedFrame
}

// connectPeer adopts a connection whose data channel opened, as the
// shared peer manager and room.ts's onPeer do together.
func (r *Room) connectPeer(p *peer, id string) {
	if r.left {
		p.destroy()
		return
	}
	st := r.state(id)
	if m := r.members[id]; m != nil && m.p != p {
		if m.p.health() != "stale" {
			p.destroy()
			return
		}
		r.exitPeer(m, true)
	}
	if st.answering != nil && st.answering != p {
		st.answering.destroy()
	}
	r.resetAnswering(st)
	st.connected, st.unhealthySince = p, time.Time{}
	r.resetOffer(st)
	m := &member{id: id, p: p, initiator: r.selfID < id, incoming: map[uint32]*incoming{}}
	r.members[id] = m
	p.onClose = func() {
		if r.members[id] == m {
			r.exitPeer(m, true)
		}
		r.disconnectPeer(p, id)
	}
	_ = p.send(presenceFrame(r.token, true))
	r.startHandshake(m)
	p.setData(func(data []byte) { r.receiveFrame(m, data) })
}

// exitPeer forgets a member and closes its connection; announce reports
// the departure of a peer that had joined.
func (r *Room) exitPeer(m *member, announce bool) {
	if r.members[m.id] != m {
		return
	}
	delete(r.members, m.id)
	if m.hsTimer != nil {
		m.hsTimer.Stop()
	}
	for _, in := range m.incoming {
		r.release(m, in)
	}
	for _, out := range r.outgoing[m.id] {
		out.fail(errPeerGone)
	}
	delete(r.outgoing, m.id)
	m.queued = nil
	if st := r.states[m.id]; st != nil && st.connected == m.p {
		st.connected = nil
	}
	m.p.destroy()
	if m.active && announce && r.onLeave != nil {
		leave, id := r.onLeave, m.id
		r.out.post(func() { leave(id) })
	}
}

// kick says goodbye to a peer that broke the protocol and drops it.
func (r *Room) kick(m *member, reason string) {
	if r.members[m.id] != m || m.failed {
		return
	}
	m.failed = true
	r.logf("Trystero: %s; disconnecting peer %s", reason, m.id)
	_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_leave", FormatString, nil)))
	r.exitPeer(m, true)
}

func (r *Room) receiveFrame(m *member, data []byte) {
	if r.members[m.id] != m {
		return
	}
	f, ok := unwrapRoomFrame(data)
	if !ok {
		r.kick(m, "invalid or incompatible room frame")
		return
	}
	if f.token != r.token {
		// Another room of the app on a shared connection: not ours.
		return
	}
	if f.presence {
		if !f.present {
			r.exitPeer(m, true)
		}
		return
	}
	r.receiveAction(m, f.payload)
}

// ----------------------------------------------------------------- handshake

type passwordMessage struct {
	Kind      string `json:"__trystero_pw"`
	Challenge string `json:"c,omitempty"`
	Hash      string `json:"h,omitempty"`
}

func (r *Room) startHandshake(m *member) {
	m.hsTimer = time.AfterFunc(handshakeTimeout, func() {
		r.loop.post(func() {
			if r.members[m.id] == m && !m.active {
				r.kick(m, fmt.Sprintf("handshake failed: handshake timed out after %dms", handshakeTimeout.Milliseconds()))
			}
		})
	})
	if r.cfg.Password == "" {
		r.passHandshake(m)
		return
	}
	if m.initiator {
		m.challenge = genID(36)
		data, _ := marshalJSON(passwordMessage{Kind: "challenge", Challenge: m.challenge})
		_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_hsdata", FormatJSON, data)))
	}
}

func (r *Room) receiveHandshake(m *member, data []byte) {
	if m.active || m.passed {
		return
	}
	var msg passwordMessage
	_ = json.Unmarshal(data, &msg)
	wrong := "handshake failed: Trystero: incorrect password for overlapping room"
	if m.initiator {
		if msg.Kind != "response" || msg.Hash != handshakeHash(m.challenge, r.cfg.Password, r.appID, r.roomID) {
			r.kick(m, wrong)
			return
		}
		r.passHandshake(m)
		return
	}
	if msg.Kind != "challenge" || msg.Challenge == "" {
		r.kick(m, wrong)
		return
	}
	reply, _ := marshalJSON(passwordMessage{Kind: "response", Hash: handshakeHash(msg.Challenge, r.cfg.Password, r.appID, r.roomID)})
	_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_hsdata", FormatJSON, reply)))
	r.passHandshake(m)
}

func (r *Room) passHandshake(m *member) {
	m.passed = true
	_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_hsready", FormatString, nil)))
	r.activate(m)
}

func (r *Room) activate(m *member) {
	if m.active || !m.passed || !m.remoteReady {
		return
	}
	m.active = true
	m.hsTimer.Stop()
	if r.onJoin != nil {
		join, id := r.onJoin, m.id
		r.out.post(func() { join(id) })
	}
}

// ------------------------------------------------------------------- actions

// action is a registered action type. Internal ones (Trystero's "@_…")
// run on the room queue; public ones go to the app.
type action struct {
	typ                 string
	receiveWhilePending bool
	maxPayload          int
	internal            func(m *member, data []byte)
	public              func(Message)
}

func (r *Room) registerInternal() {
	internal := func(typ string, pending bool, maxPayload int, fn func(*member, []byte)) {
		r.actions[typ] = &action{typ: typ, receiveWhilePending: pending, maxPayload: maxPayload, internal: fn}
	}
	nothing := func(*member, []byte) {}
	internal("@_ping", false, 0, func(m *member, _ []byte) {
		_ = m.p.send(wrapRoomFrame(r.token, r.inline("@_pong", FormatString, nil)))
	})
	internal("@_pong", false, 0, nothing)
	// Media renegotiation and metadata: this side sends no media and
	// ignores a peer's.
	internal("@_signal", false, 0, nothing)
	internal("@_stream", false, 65536, nothing)
	internal("@_track", false, 65536, nothing)
	internal("@_response", false, 0, nothing)
	internal("@_leave", true, 0, func(m *member, _ []byte) { r.exitPeer(m, true) })
	internal("@_hsdata", true, 65536, r.receiveHandshake)
	internal("@_hsready", true, 0, func(m *member, _ []byte) {
		if !m.active {
			m.remoteReady = true
			r.activate(m)
		}
	})
}

func (r *Room) inline(typ string, format byte, payload []byte) []byte {
	frame, _, _ := encodeStart(typ, format, payload, nil, 0)
	return frame
}

// queuedFrame is a message or an offer for an action not made yet, kept
// for it as Trystero's receiver keeps it, until it expires.
type queuedFrame struct {
	f  actionFrame
	at time.Time
}

const maxPendingPerPeer = 64

func (r *Room) receiveAction(m *member, data []byte) {
	f, err := decodeAction(data)
	if err != nil {
		r.kick(m, err.Error())
		return
	}
	switch f.kind {
	case kindAccepted:
		if out := r.outgoing[m.id][f.id]; out != nil {
			out.accept()
			for _, other := range r.outgoing[m.id] {
				other.touch()
			}
		}
		return
	case kindRejected:
		if out := r.outgoing[m.id][f.id]; out != nil {
			out.reject(f.reason)
		}
		return
	case kindCancelled:
		if in := m.incoming[f.id]; in != nil {
			r.release(m, in)
			r.drain(m)
		}
		return
	case kindChunk:
		r.receiveChunk(m, f)
		return
	}
	a := r.actions[f.typ]
	if !m.active && (a == nil || !a.receiveWhilePending) {
		return
	}
	if a == nil {
		r.queue(m, f)
		return
	}
	r.admit(m, a, f)
}

func (r *Room) queue(m *member, f actionFrame) {
	now := time.Now()
	kept := m.queued[:0]
	for _, q := range m.queued {
		if now.Sub(q.at) < transferTimeout {
			kept = append(kept, q)
		} else if q.f.kind == kindOffer {
			_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindRejected, q.f.id, "action transfer timed out")))
		}
	}
	m.queued = kept
	if len(m.queued)+len(m.incoming) >= maxPendingPerPeer {
		if f.kind == kindOffer {
			_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindRejected, f.id, "too many pending transfers")))
		}
		return
	}
	if f.kind == kindInline {
		f.payload = append([]byte(nil), f.payload...)
	}
	m.queued = append(m.queued, &queuedFrame{f: f, at: now})
}

func (r *Room) flushQueued(m *member, typ string) {
	a := r.actions[typ]
	var rest []*queuedFrame
	queued := m.queued
	m.queued = nil
	for _, q := range queued {
		if q.f.typ == typ && time.Since(q.at) < transferTimeout {
			r.admit(m, a, q.f)
		} else if q.f.typ != typ {
			rest = append(rest, q)
		}
	}
	m.queued = append(rest, m.queued...)
}

func (r *Room) limit(a *action) int {
	if a.maxPayload > 0 {
		return min(a.maxPayload, defaultMaxReceiveBytes)
	}
	return defaultMaxReceiveBytes
}

func (r *Room) admit(m *member, a *action, f actionFrame) {
	if f.kind == kindInline {
		if f.size > r.limit(a) {
			return
		}
		r.deliver(m, a, f.format, f.payload)
		return
	}
	if m.incoming[f.id] != nil {
		r.kick(m, "duplicate action offer")
		return
	}
	if f.size > r.limit(a) {
		_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindRejected, f.id, "payload exceeds receiver size limit")))
		return
	}
	if len(m.incoming) >= maxPendingPerPeer {
		_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindRejected, f.id, "too many pending transfers")))
		return
	}
	in := &incoming{id: f.id, typ: f.typ, format: f.format, size: f.size, lastSeen: time.Now()}
	m.incoming[f.id] = in
	m.waiting = append(m.waiting, in)
	in.timer = time.AfterFunc(transferTimeout, func() { r.loop.post(func() { r.expire(m, in) }) })
	r.drain(m)
}

func (r *Room) deliver(m *member, a *action, format byte, payload []byte) {
	if format == FormatJSON && !json.Valid(payload) {
		r.kick(m, "invalid action payload")
		return
	}
	if a.internal != nil {
		a.internal(m, payload)
		return
	}
	if a.public != nil {
		handler, msg := a.public, Message{PeerID: m.id, Format: format, Data: append([]byte(nil), payload...)}
		r.out.post(func() { handler(msg) })
	}
}

// incoming is a chunked transfer from a peer. A peer sends one at a time:
// the next waits for this side's acceptance.
type incoming struct {
	id       uint32
	typ      string
	format   byte
	size     int
	data     []byte
	offset   int
	lastSeen time.Time
	timer    *time.Timer
}

// drain accepts the next waiting transfer of a peer when none is active.
func (r *Room) drain(m *member) {
	if r.members[m.id] != m || m.receiving != nil {
		return
	}
	for len(m.waiting) > 0 {
		in := m.waiting[0]
		if r.reserved+in.size > defaultMaxReceiveBytes {
			return
		}
		m.waiting = m.waiting[1:]
		m.receiving = in
		r.reserved += in.size
		in.lastSeen = time.Now()
		_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindAccepted, in.id, "")))
		return
	}
}

func (r *Room) release(m *member, in *incoming) {
	if m.incoming[in.id] != in {
		return
	}
	delete(m.incoming, in.id)
	if in.timer != nil {
		in.timer.Stop()
	}
	if m.receiving == in {
		m.receiving = nil
		r.reserved -= in.size
	}
	for i, w := range m.waiting {
		if w == in {
			m.waiting = append(m.waiting[:i], m.waiting[i+1:]...)
			break
		}
	}
	in.data = nil
}

func (r *Room) expire(m *member, in *incoming) {
	if m.incoming[in.id] != in {
		return
	}
	if remaining := transferTimeout - time.Since(in.lastSeen); remaining > 0 {
		in.timer = time.AfterFunc(remaining, func() { r.loop.post(func() { r.expire(m, in) }) })
		return
	}
	r.release(m, in)
	_ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindRejected, in.id, "action transfer timed out")))
	r.drain(m)
}

func (r *Room) receiveChunk(m *member, f actionFrame) {
	in := m.incoming[f.id]
	if in == nil {
		return
	}
	want := min(chunkSize, in.size-in.offset)
	if m.receiving != in || math.IsNaN(f.offset) || f.offset != float64(in.offset) || len(f.payload) == 0 || len(f.payload) != want {
		r.kick(m, "invalid action chunk offset or length")
		return
	}
	if in.data == nil {
		in.data = make([]byte, in.size)
	}
	copy(in.data[in.offset:], f.payload)
	in.offset += len(f.payload)
	now := time.Now()
	in.lastSeen = now
	for _, w := range m.waiting {
		w.lastSeen = now
	}
	if in.offset < in.size {
		return
	}
	data := in.data
	r.release(m, in)
	if a := r.actions[in.typ]; a != nil {
		r.deliver(m, a, in.format, data)
	}
	r.drain(m)
}

// outgoing is a chunked transfer to a peer, waiting for its acceptance.
type outgoing struct {
	accepted chan struct{}
	failed   chan error
	seen     chan struct{}
	done     bool
}

func (o *outgoing) accept() {
	select {
	case o.accepted <- struct{}{}:
	default:
	}
}

func (o *outgoing) reject(reason string) { o.fail(errors.New("Trystero: " + reason)) }

func (o *outgoing) fail(err error) {
	select {
	case o.failed <- err:
	default:
	}
}

func (o *outgoing) touch() {
	select {
	case o.seen <- struct{}{}:
	default:
	}
}

// Send sends an action's payload to targets, or to every joined peer when
// there are none. It returns once each frame is handed to the channels,
// as Trystero's send resolves; a large payload first waits for each peer
// to accept it. Targets not in the room are skipped. Do not call it from
// the room's own handlers' goroutine while that blocks it: it waits for
// the room.
func (r *Room) Send(typ string, format byte, payload []byte, targets ...string) error {
	if format > FormatBinary {
		return errors.New("invalid format")
	}
	type plan struct {
		members []*member
		ids     []uint32
		start   [][]byte
		inline  bool
		err     error
	}
	p := call(r.loop, func() plan {
		var pl plan
		if r.left {
			pl.err = errors.New("room left")
			return pl
		}
		if len(targets) == 0 {
			for _, m := range r.members {
				if m.active {
					pl.members = append(pl.members, m)
				}
			}
		} else {
			seen := map[string]bool{}
			for _, id := range targets {
				if m := r.members[id]; m != nil && m.active && !seen[id] {
					seen[id] = true
					pl.members = append(pl.members, m)
				} else if m == nil || !m.active {
					r.logf("Trystero: no peer with id %s found", id)
				}
			}
		}
		for range pl.members {
			id := r.nextID
			frame, inline, err := encodeStart(typ, format, payload, nil, id)
			if err != nil {
				pl.err = err
				return pl
			}
			pl.inline = inline
			if !inline {
				r.nextID++
			}
			pl.ids = append(pl.ids, id)
			pl.start = append(pl.start, wrapRoomFrame(r.token, frame))
		}
		return pl
	})
	if p.err != nil {
		return p.err
	}
	errs := make(chan error, len(p.members))
	for i, m := range p.members {
		go func() {
			if p.inline {
				errs <- m.p.send(p.start[i])
				return
			}
			errs <- r.sendChunked(m, p.ids[i], p.start[i], payload)
		}()
	}
	var first error
	for range p.members {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	return first
}

// SendJSON sends v encoded as JSON.
func (r *Room) SendJSON(typ string, v any, targets ...string) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}
	return r.Send(typ, FormatJSON, data, targets...)
}

func (r *Room) sendChunked(m *member, id uint32, start, payload []byte) error {
	out := &outgoing{accepted: make(chan struct{}, 1), failed: make(chan error, 1), seen: make(chan struct{}, 1)}
	ok := call(r.loop, func() bool {
		if r.members[m.id] != m {
			return false
		}
		transfers := r.outgoing[m.id]
		if transfers == nil {
			transfers = map[uint32]*outgoing{}
			r.outgoing[m.id] = transfers
		}
		transfers[id] = out
		return true
	})
	if !ok {
		return errPeerGone
	}
	defer r.loop.post(func() {
		if transfers := r.outgoing[m.id]; transfers != nil && transfers[id] == out {
			delete(transfers, id)
			if len(transfers) == 0 {
				delete(r.outgoing, m.id)
			}
		}
	})
	cancel := func() { _ = m.p.send(wrapRoomFrame(r.token, controlFrame(kindCancelled, id, ""))) }
	if err := m.p.send(start); err != nil {
		return err
	}
	timer := time.NewTimer(transferTimeout)
	defer timer.Stop()
wait:
	for {
		select {
		case <-out.accepted:
			break wait
		case <-out.seen:
			timer.Reset(transferTimeout)
		case err := <-out.failed:
			return err
		case <-timer.C:
			cancel()
			return errors.New("Trystero: action offer timed out")
		}
	}
	for offset := 0; offset < len(payload); offset += chunkSize {
		select {
		case err := <-out.failed:
			if !errors.Is(err, errPeerGone) && !strings.HasPrefix(err.Error(), "Trystero: ") {
				cancel()
			}
			return err
		default:
		}
		end := min(offset+chunkSize, len(payload))
		if err := m.p.send(wrapRoomFrame(r.token, chunkFrame(id, offset, payload[offset:end]))); err != nil {
			return err
		}
		r.loop.post(func() {
			for _, other := range r.outgoing[m.id] {
				other.touch()
			}
		})
	}
	return nil
}

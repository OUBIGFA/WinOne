package trystero

import (
	"encoding/json"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// The WebTorrent tracker client of @trystero-p2p/torrent: each tracker
// hears this side's offers in announce messages and passes them, one per
// peer, to others in the room's swarm, whose answers come back the same
// way. The tracker never sees a description in clear: offers and answers
// are encrypted with the room's password.

const (
	defaultAnnounce   = 10 * time.Second
	offerRetention    = 120 * time.Second
	requestedPeers    = 3
	offersPerAnnounce = 1
	defaultRetry      = 3333 * time.Millisecond
	maxRetry          = 60 * time.Second
	dialTimeout       = 10 * time.Second
)

// announceWarmup are strategy.ts's first intervals, so peers that join
// together meet within seconds rather than at the next interval.
var announceWarmup = []time.Duration{233 * time.Millisecond, 533 * time.Millisecond, 1333 * time.Millisecond}

// pooledOffer is an offer announced and not yet answered, shared by the
// room's trackers.
type pooledOffer struct {
	peer    *peer
	sdp     string
	created time.Time
}

type relay struct {
	room *Room
	url  string

	mu     sync.Mutex
	conn   *websocket.Conn
	closed bool
	quit   chan struct{}
	once   sync.Once

	// Room queue only.
	opened     bool
	announceMs time.Duration
	attempts   int
	timer      *time.Timer
}

// run keeps the tracker's socket open, reconnecting after a random part
// of a period that doubles up to a minute, as utils.ts's makeSocket.
func (rl *relay) run() {
	period := defaultRetry
	dialer := websocket.Dialer{HandshakeTimeout: dialTimeout, Proxy: nil}
	for {
		rl.mu.Lock()
		closed := rl.closed
		rl.mu.Unlock()
		if closed {
			return
		}
		conn, _, err := dialer.Dial(rl.url, nil)
		if err == nil {
			rl.mu.Lock()
			if rl.closed {
				rl.mu.Unlock()
				_ = conn.Close()
				return
			}
			rl.conn = conn
			rl.mu.Unlock()
			period = defaultRetry
			rl.room.loop.post(rl.onOpen)
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					break
				}
				rl.room.loop.post(func() { rl.receive(data) })
			}
			rl.mu.Lock()
			rl.conn = nil
			rl.mu.Unlock()
			_ = conn.Close()
		}
		delay := time.Duration(rand.Float64() * float64(period))
		period = min(period*2, maxRetry)
		select {
		case <-rl.quit:
			return
		case <-time.After(delay):
		}
	}
}

func (rl *relay) stop() {
	rl.once.Do(func() {
		rl.mu.Lock()
		rl.closed = true
		if rl.conn != nil {
			_ = rl.conn.Close()
		}
		close(rl.quit)
		rl.mu.Unlock()
	})
	if rl.timer != nil {
		rl.timer.Stop()
	}
}

// send writes a message when the socket is open, and drops it otherwise,
// as the browser client does.
func (rl *relay) send(message []byte) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.conn == nil || rl.closed {
		return
	}
	_ = rl.conn.SetWriteDeadline(time.Now().Add(dialTimeout))
	_ = rl.conn.WriteMessage(websocket.TextMessage, message)
}

func (rl *relay) onOpen() {
	r := rl.room
	if r.left {
		return
	}
	if !rl.opened {
		rl.opened = true
		r.queueAnnounce(rl)
		return
	}
	// A reconnection announces at once and keeps the schedule.
	r.announce(rl, nil)
}

// queueAnnounce announces and schedules the next announcement.
func (r *Room) queueAnnounce(rl *relay) {
	if r.left {
		return
	}
	r.announce(rl, func() {
		if r.left {
			return
		}
		delay := rl.announceMs
		if rl.attempts < len(announceWarmup) {
			delay = min(delay, announceWarmup[rl.attempts])
		}
		rl.attempts++
		rl.timer = time.AfterFunc(delay, func() { r.loop.post(func() { r.queueAnnounce(rl) }) })
	})
}

// announce sends the room's outstanding offer to a tracker.
func (r *Room) announce(rl *relay, done func()) {
	r.ensureOffers(func(ok bool) {
		if r.left {
			return
		}
		if ok {
			offers := []map[string]any{}
			for id, o := range r.offers {
				offers = append(offers, map[string]any{"offer_id": id, "offer": map[string]string{"type": "offer", "sdp": o.sdp}})
			}
			message, _ := marshalJSON(map[string]any{"action": "announce", "info_hash": r.hash, "peer_id": r.selfID,
				"numwant": requestedPeers, "offers": offers})
			rl.send(message)
		}
		if done != nil {
			done()
		}
	})
}

// ensureOffers keeps offersPerAnnounce offers ready, making them one batch
// at a time, and drops those older than offerRetention.
func (r *Room) ensureOffers(ready func(bool)) {
	now := time.Now()
	for id, o := range r.offers {
		if now.Sub(o.created) > offerRetention {
			o.peer.destroy()
			delete(r.offers, id)
		}
	}
	if len(r.offers) >= offersPerAnnounce {
		ready(true)
		return
	}
	r.waiters = append(r.waiters, ready)
	if r.generating {
		return
	}
	r.generating = true
	go func() {
		var p *peer
		var sealed string
		var err error
		// One retry, as the offer manager's.
		for range 2 {
			p, sealed, err = r.makeOffer()
			if err == nil {
				break
			}
		}
		r.loop.post(func() {
			r.generating = false
			ok := err == nil && !r.left
			if ok {
				r.offers[genID(20)] = &pooledOffer{peer: p, sdp: sealed, created: time.Now()}
			} else if p != nil {
				p.destroy()
			}
			if err != nil {
				r.logf("Trystero: announce failed - %v", err)
			}
			waiters := r.waiters
			r.waiters = nil
			for _, w := range waiters {
				w(ok)
			}
		})
	}()
}

// makeOffer prepares an offering connection and its encrypted offer. It
// blocks while candidates gather.
func (r *Room) makeOffer() (*peer, string, error) {
	p, err := r.newPeer(true)
	if err != nil {
		return nil, "", err
	}
	sdp, err := p.offer()
	if err == nil {
		var sealed string
		sealed, err = encryptSignal(r.aead, sdp)
		if err == nil {
			return p, sealed, nil
		}
	}
	r.loop.post(p.destroy)
	return nil, "", err
}

// trackerMessage is what a tracker sends: its response to an announce, or
// a peer's offer or answer passed on.
type trackerMessage struct {
	Failure  string          `json:"failure reason"`
	Warning  string          `json:"warning message"`
	Interval float64         `json:"interval"`
	InfoHash string          `json:"info_hash"`
	PeerID   string          `json:"peer_id"`
	OfferID  string          `json:"offer_id"`
	Offer    *sessionMessage `json:"offer"`
	Answer   *sessionMessage `json:"answer"`
}

type sessionMessage struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func (rl *relay) receive(data []byte) {
	r := rl.room
	if r.left {
		return
	}
	var msg trackerMessage
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	if msg.Failure != "" {
		r.logf("Trystero: torrent tracker failure from %s - %s", rl.url, msg.Failure)
		return
	}
	if msg.Warning != "" {
		r.logf("Trystero: torrent tracker warning from %s - %s", rl.url, msg.Warning)
	}
	ours := msg.InfoHash == r.hash
	if msg.Interval > 0 && ours {
		rl.announceMs = min(max(time.Duration(msg.Interval*float64(time.Second)), defaultAnnounce), offerRetention)
	}
	if (msg.Offer == nil && msg.Answer == nil) || !ours || msg.OfferID == "" || msg.PeerID == r.selfID {
		return
	}
	kind := "answer"
	if msg.Offer != nil {
		kind = "offer"
	}
	key := r.rootTopic + ":" + kind + ":" + msg.OfferID + ":" + msg.PeerID
	now := time.Now()
	if last, ok := r.signals[key]; ok && now.Sub(last) < signalDedupeWindow {
		return
	}
	r.signals[key] = now
	for k, at := range r.signals {
		if now.Sub(at) > 6*signalDedupeWindow {
			delete(r.signals, k)
		}
	}
	switch {
	case msg.Offer != nil && msg.PeerID != "":
		if msg.Offer.SDP == "" || !r.signal(msg.PeerID) {
			return
		}
		r.handleOffer(rl, msg.PeerID, msg.Offer.SDP, msg.OfferID)
	case msg.Answer != nil && msg.PeerID != "":
		o := r.offers[msg.OfferID]
		if o == nil {
			return
		}
		delete(r.offers, msg.OfferID)
		if msg.Answer.SDP == "" || !r.signal(msg.PeerID) {
			o.peer.destroy()
		} else {
			r.handleAnswer(msg.PeerID, msg.Answer.SDP, msg.OfferID, o.peer)
		}
		r.announce(rl, nil)
	}
}

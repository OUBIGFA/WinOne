package trystero

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

// The WebRTC side of Trystero's peer.ts, on pion: one data channel named
// "data" opened by the side that offers, binary frames, and signalling
// without trickle ICE (the torrent strategy's default), so an offer or an
// answer carries the candidates gathered before it is sent.

const (
	iceTimeout             = 15 * time.Second
	iceCandidateSettle     = 150 * time.Millisecond
	disconnectedCloseDelay = 5 * time.Second
	// bufferedLowThreshold is the data channel's bufferedAmountLowThreshold;
	// a sender waits for the buffer to drain below it before each frame.
	bufferedLowThreshold = 65535
	backpressureTimeout  = 10 * time.Second
	maxPendingFrames     = 64
)

// DefaultICEServers are Trystero's STUN servers, used unless the RTC
// configuration names its own.
var DefaultICEServers = []ICEServer{
	{URLs: []string{"stun:stun.l.google.com:19302"}},
	{URLs: []string{"stun:stun1.l.google.com:19302"}},
	{URLs: []string{"stun:stun2.l.google.com:19302"}},
	{URLs: []string{"stun:stun.cloudflare.com:3478"}},
}

// peer is one RTCPeerConnection with its data channel. Its handlers run on
// the room's queue and are set there.
type peer struct {
	room      *Room
	pc        *webrtc.PeerConnection
	dc        atomic.Pointer[webrtc.DataChannel]
	initiator bool
	dead      atomic.Bool
	closing   chan struct{}
	closeOnce sync.Once

	// Room queue only.
	onConnect func()
	onClose   func()
	onData    func([]byte)
	pending   [][]byte
	connected bool
	emitted   bool
	timer     *time.Timer
}

func (r *Room) newPeer(initiator bool) (*peer, error) {
	pc, err := r.api.NewPeerConnection(r.rtc)
	if err != nil {
		return nil, err
	}
	p := &peer{room: r, pc: pc, initiator: initiator, closing: make(chan struct{})}
	if initiator {
		dc, err := pc.CreateDataChannel("data", nil)
		if err != nil {
			_ = pc.Close()
			return nil, err
		}
		p.bind(dc)
	} else {
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			if p.dc.Load() == nil {
				p.bind(dc)
			}
		})
	}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		r.loop.post(func() { p.connectionState(state) })
	})
	return p, nil
}

func (p *peer) bind(dc *webrtc.DataChannel) {
	p.dc.Store(dc)
	dc.SetBufferedAmountLowThreshold(bufferedLowThreshold)
	r := p.room
	dc.OnOpen(func() {
		r.loop.post(func() {
			if !p.emitted && !p.connected {
				p.connected = true
				if p.onConnect != nil {
					p.onConnect()
				}
			}
		})
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Trystero sends ArrayBuffers only and drops a peer for anything
		// else or for a frame over the room limit.
		if msg.IsString || len(msg.Data) > maxRoomFrameBytes {
			r.loop.post(func() { p.destroy() })
			return
		}
		data := msg.Data
		r.loop.post(func() { p.receive(data) })
	})
	dc.OnClose(func() { r.loop.post(p.emitClose) })
}

func (p *peer) receive(data []byte) {
	if p.emitted {
		return
	}
	if p.onData != nil {
		p.onData(data)
		return
	}
	if len(p.pending) >= maxPendingFrames {
		p.destroy()
		return
	}
	p.pending = append(p.pending, data)
}

// setData installs the frame handler and gives it what came before.
func (p *peer) setData(fn func([]byte)) {
	p.onData = fn
	pending := p.pending
	p.pending = nil
	for _, data := range pending {
		if p.emitted || p.onData == nil {
			return
		}
		p.onData(data)
	}
}

func (p *peer) connectionState(state webrtc.PeerConnectionState) {
	switch state {
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		p.emitClose()
	case webrtc.PeerConnectionStateDisconnected:
		if p.timer == nil {
			p.timer = time.AfterFunc(disconnectedCloseDelay, func() {
				p.room.loop.post(func() {
					p.timer = nil
					if p.pc.ConnectionState() == webrtc.PeerConnectionStateDisconnected {
						p.emitClose()
					}
				})
			})
		}
	default:
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
	}
}

// emitClose reports the end of the connection once, on the room queue.
func (p *peer) emitClose() {
	if p.emitted {
		return
	}
	p.emitted = true
	p.pending = nil
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.shutdown()
	if p.onClose != nil {
		p.onClose()
	}
}

// shutdown closes the connection off the queue: pion's Close waits for
// its transports.
func (p *peer) shutdown() {
	p.closeOnce.Do(func() {
		p.dead.Store(true)
		close(p.closing)
		go func() {
			if dc := p.dc.Load(); dc != nil {
				_ = dc.Close()
			}
			_ = p.pc.Close()
		}()
	})
}

// destroy ends the connection; its close handler runs.
func (p *peer) destroy() { p.emitClose() }

// health is shared-peer.ts's view of a connected peer: live while its
// channel is open, stale once anything is closed or failed, and transient
// in between.
func (p *peer) health() string {
	if p.dead.Load() {
		return "stale"
	}
	switch p.pc.ConnectionState() {
	case webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateFailed:
		return "stale"
	}
	switch p.pc.ICEConnectionState() {
	case webrtc.ICEConnectionStateClosed, webrtc.ICEConnectionStateFailed:
		return "stale"
	}
	dc := p.dc.Load()
	if dc == nil {
		return "transient"
	}
	switch dc.ReadyState() {
	case webrtc.DataChannelStateOpen:
		return "live"
	case webrtc.DataChannelStateClosing, webrtc.DataChannelStateClosed:
		return "stale"
	}
	return "transient"
}

var errPeerGone = errors.New("peer disconnected")

// send writes one frame, first waiting for the channel to drain below its
// threshold, as action-wire.ts does before each frame.
func (p *peer) send(frame []byte) error {
	dc := p.dc.Load()
	if dc == nil || p.dead.Load() {
		return errPeerGone
	}
	if dc.BufferedAmount() > bufferedLowThreshold {
		deadline := time.Now().Add(backpressureTimeout)
		for dc.BufferedAmount() > bufferedLowThreshold {
			if p.dead.Load() || dc.ReadyState() != webrtc.DataChannelStateOpen {
				return errPeerGone
			}
			if time.Now().After(deadline) {
				return errors.New("data channel stopped draining")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := dc.Send(frame); err != nil {
		return errPeerGone
	}
	return nil
}

// offer makes the initial offer, with the candidates gathered for it. It
// blocks: call it off the room queue.
func (p *peer) offer() (string, error) {
	return p.describe(func() error {
		offer, err := p.pc.CreateOffer(nil)
		if err != nil {
			return err
		}
		return p.pc.SetLocalDescription(offer)
	})
}

// answer applies a remote offer and returns the answer. It blocks.
func (p *peer) answer(sdp string) (string, error) {
	return p.describe(func() error {
		if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
			return err
		}
		answer, err := p.pc.CreateAnswer(nil)
		if err != nil {
			return err
		}
		return p.pc.SetLocalDescription(answer)
	})
}

// accept applies the remote answer to this side's offer. It blocks.
func (p *peer) accept(sdp string) error {
	return p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
}

// describe sets the local description and waits for ICE gathering as
// peer.ts's waitForIceGathering: until it completes, or 150 ms after the
// last candidate of the kind the configuration expects, or 15 s.
func (p *peer) describe(set func() error) (string, error) {
	complete := make(chan struct{})
	hit := make(chan struct{}, 1)
	var once sync.Once
	p.pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			once.Do(func() { close(complete) })
			return
		}
		if p.room.expects(c.Typ) {
			select {
			case hit <- struct{}{}:
			default:
			}
		}
	})
	if err := set(); err != nil {
		return "", err
	}
	if p.pc.ICEGatheringState() == webrtc.ICEGatheringStateComplete {
		once.Do(func() { close(complete) })
	}
	deadline := time.NewTimer(iceTimeout)
	defer deadline.Stop()
	var settle <-chan time.Time
wait:
	for {
		select {
		case <-complete:
			break wait
		case <-hit:
			settle = time.After(iceCandidateSettle)
		case <-settle:
			break wait
		case <-deadline.C:
			break wait
		case <-p.closing:
			return "", errPeerGone
		}
	}
	desc := p.pc.LocalDescription()
	if desc == nil {
		return "", errors.New("no local description")
	}
	return desc.SDP, nil
}

// candidatePattern is which candidates peer.ts waits for: relay ones when a
// TURN server is configured, server-reflexive ones with STUN, else any.
type candidatePattern int

const (
	anyCandidate candidatePattern = iota
	reflexiveCandidate
	relayCandidate
)

var (
	turnURL = regexp.MustCompile(`(?i)^turns?:`)
	stunURL = regexp.MustCompile(`(?i)^stuns?:`)
)

func patternFor(servers []ICEServer) candidatePattern {
	has := func(re *regexp.Regexp) bool {
		for _, s := range servers {
			for _, u := range s.URLs {
				if re.MatchString(strings.TrimSpace(u)) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has(turnURL):
		return relayCandidate
	case has(stunURL):
		return reflexiveCandidate
	}
	return anyCandidate
}

func (r *Room) expects(t webrtc.ICECandidateType) bool {
	switch r.pattern {
	case relayCandidate:
		return t == webrtc.ICECandidateTypeRelay
	case reflexiveCandidate:
		return t == webrtc.ICECandidateTypeSrflx || t == webrtc.ICECandidateTypeRelay || t == webrtc.ICECandidateTypePrflx
	}
	return true
}

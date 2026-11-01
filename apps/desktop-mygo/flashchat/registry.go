package flashchat

import (
	"crypto/rand"
	"fmt"
	"sync"
)

// Registry holds the rooms a host has open: one group room and a
// direct-message room per peer (ChatRoomRegistry). Conversations outlive
// the view that shows them, so the registry owns them. It also forwards
// the group room's peer arrivals and departures to the conversation each
// concerns, as only the group room hears them.
type Registry struct {
	mu     sync.Mutex
	group  *ChatRoom
	direct map[string]*ChatRoom
	offs   []func()
}

// NewRegistry makes an empty registry.
func NewRegistry() *Registry { return &Registry{direct: map[string]*ChatRoom{}} }

// Group returns the group room.
func (g *Registry) Group() *ChatRoom {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.group
}

// SetGroup adopts room as the group room (nil forgets it).
func (g *Registry) SetGroup(room *ChatRoom) {
	g.mu.Lock()
	offs := g.offs
	g.offs, g.group = nil, room
	g.mu.Unlock()
	for _, off := range offs {
		off()
	}
	if room == nil {
		return
	}
	join := room.On(PeerJoin, func(e Event) {
		if dm := g.Direct(e.PeerID); dm != nil {
			dm.NotifyPeerJoined(e.PeerID)
		}
	})
	leave := room.On(PeerLeave, func(e Event) {
		if dm := g.Direct(e.PeerID); dm != nil {
			dm.NotifyPeerLeft(e.PeerID)
		}
	})
	g.mu.Lock()
	g.offs = []func(){join, leave}
	g.mu.Unlock()
}

// Direct returns the conversation with a peer, nil when none is open.
func (g *Registry) Direct(peerID string) *ChatRoom {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.direct[peerID]
}

// SetDirect records the conversation with a peer.
func (g *Registry) SetDirect(peerID string, room *ChatRoom) {
	g.mu.Lock()
	g.direct[peerID] = room
	g.mu.Unlock()
}

// ClearDirect leaves and forgets every conversation.
func (g *Registry) ClearDirect() {
	g.mu.Lock()
	rooms := g.direct
	g.direct = map[string]*ChatRoom{}
	g.mu.Unlock()
	for _, room := range rooms {
		room.Leave()
	}
}

// NewUUID returns a random version 4 UUID, as crypto.randomUUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

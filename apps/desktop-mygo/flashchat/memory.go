package flashchat

import (
	"encoding/json"
	"sync"
)

// MemoryNetwork connects MemoryTransports in one process, standing in for
// one Trystero room in tests (InMemoryTransport.ts): members are
// announced to each other once both listen for joins, and actions are
// delivered asynchronously, in order, as over a data channel.
type MemoryNetwork struct {
	mu          sync.Mutex
	members     map[string]*MemoryTransport
	listening   map[string]bool
	connections map[string]map[string]bool
}

// NewMemoryNetwork makes an empty network.
func NewMemoryNetwork() *MemoryNetwork {
	return &MemoryNetwork{members: map[string]*MemoryTransport{}, listening: map[string]bool{}, connections: map[string]map[string]bool{}}
}

// MemoryTransport is one peer of a MemoryNetwork.
type MemoryTransport struct {
	net  *MemoryNetwork
	id   string
	hub  *hub
	in   chan func()
	done chan struct{}
	once sync.Once
	// FailSend, when set, fails the sends of an action name.
	FailSend func(name string) error
}

// Join adds a peer to the network.
func (n *MemoryNetwork) Join(id string) *MemoryTransport {
	t := &MemoryTransport{net: n, id: id, hub: newHub(), in: make(chan func(), 4096), done: make(chan struct{})}
	go func() {
		for {
			select {
			case fn := <-t.in:
				fn()
			case <-t.done:
				return
			}
		}
	}()
	n.mu.Lock()
	n.members[id] = t
	n.connections[id] = map[string]bool{}
	n.mu.Unlock()
	return t
}

func (t *MemoryTransport) post(fn func()) {
	select {
	case <-t.done:
	case t.in <- fn:
	}
}

func (n *MemoryNetwork) startListening(id string) {
	n.mu.Lock()
	if n.listening[id] {
		n.mu.Unlock()
		return
	}
	n.listening[id] = true
	var joins [][2]*MemoryTransport
	for other := range n.listening {
		if other == id || n.connections[id][other] {
			continue
		}
		n.connections[id][other] = true
		n.connections[other][id] = true
		joins = append(joins, [2]*MemoryTransport{n.members[id], n.members[other]})
	}
	n.mu.Unlock()
	for _, pair := range joins {
		a, b := pair[0], pair[1]
		a.post(func() { a.hub.peerJoined(b.id) })
		b.post(func() { b.hub.peerJoined(a.id) })
	}
}

func (t *MemoryTransport) MakeAction(name string) ActionHandle {
	shared, _ := t.hub.shared(name)
	return &handle{shared: shared, send: func(payload any, target string) error {
		if t.FailSend != nil {
			if err := t.FailSend(name); err != nil {
				return err
			}
		}
		data, err := marshalJSON(payload)
		if err != nil {
			return err
		}
		t.net.mu.Lock()
		var targets []*MemoryTransport
		for id := range t.net.connections[t.id] {
			if target == "" || id == target {
				targets = append(targets, t.net.members[id])
			}
		}
		t.net.mu.Unlock()
		for _, other := range targets {
			other.post(func() {
				s, _ := other.hub.shared(name)
				s.dispatch(json.RawMessage(data), t.id)
			})
		}
		return nil
	}}
}

func (t *MemoryTransport) OnPeerJoin(hook string, fn func(string)) {
	t.hub.onPeerJoin(hook, fn, t.Peers())
	t.net.startListening(t.id)
}

func (t *MemoryTransport) OnPeerLeave(hook string, fn func(string)) { t.hub.onPeerLeave(hook, fn) }

func (t *MemoryTransport) Peers() []string {
	t.net.mu.Lock()
	defer t.net.mu.Unlock()
	var ids []string
	for id := range t.net.connections[t.id] {
		ids = append(ids, id)
	}
	return ids
}

// Leave announces the departure to every remaining member.
func (t *MemoryTransport) Leave() {
	t.once.Do(func() {
		n := t.net
		n.mu.Lock()
		delete(n.members, t.id)
		delete(n.listening, t.id)
		delete(n.connections, t.id)
		var others []*MemoryTransport
		for _, other := range n.members {
			delete(n.connections[other.id], t.id)
			others = append(others, other)
		}
		n.mu.Unlock()
		for _, other := range others {
			other.post(func() { other.hub.peerLeft(t.id) })
		}
		close(t.done)
	})
}

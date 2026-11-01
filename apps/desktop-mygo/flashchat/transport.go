package flashchat

import (
	"encoding/json"
	"strconv"
	"sync"

	"winone/desktopmygo/flashchat/trystero"
)

// Transport is the peer surface a ChatRoom speaks over (transport/types.ts
// PeerTransport): named actions shared by every room on it, and a single
// peer-join and peer-leave slot per hook type, fanned out.
type Transport interface {
	// MakeAction returns a handle on the action name; every handle of a
	// name shares one channel but receives and disconnects on its own.
	MakeAction(name string) ActionHandle
	OnPeerJoin(hook string, fn func(peerID string))
	OnPeerLeave(hook string, fn func(peerID string))
	Peers() []string
	// Leave disconnects from the room; it is the host's decision, as
	// direct-message rooms share the group room's transport.
	Leave()
}

// ActionHandle sends and receives one action's JSON payloads.
type ActionHandle interface {
	// Send delivers payload to target, or to every peer when target is "".
	Send(payload any, target string) error
	Connect(fn func(data json.RawMessage, peerID string))
	Disconnect()
}

// HookNewPeer is the chat's join/leave hook; the others were media's.
const HookNewPeer = "NEW_PEER"

// ActionName is the wire name of an action in a namespace.
func ActionName(namespace string, action PeerAction) string {
	return namespace + "." + strconv.Itoa(int(action))
}

// hub fans peer membership and actions out to the handlers registered on
// a transport, as TrysteroTransport does over Trystero's single slots.
type hub struct {
	mu        sync.Mutex
	joins     map[string]func(string)
	leaves    map[string]func(string)
	announced map[string]map[string]bool
	actions   map[string]*sharedAction
}

type sharedAction struct {
	mu        sync.Mutex
	receivers map[*handle]func(json.RawMessage, string)
}

func newHub() *hub {
	return &hub{joins: map[string]func(string){}, leaves: map[string]func(string){}, announced: map[string]map[string]bool{},
		actions: map[string]*sharedAction{}}
}

// peerJoined tells every hook about a peer once.
func (h *hub) peerJoined(id string) {
	h.mu.Lock()
	var fns []func(string)
	for hook, fn := range h.joins {
		if h.announced[hook] == nil {
			h.announced[hook] = map[string]bool{}
		}
		h.announced[hook][id] = true
		fns = append(fns, fn)
	}
	h.mu.Unlock()
	for _, fn := range fns {
		fn(id)
	}
}

// peerLeft forgets a peer, so that it is announced again if it returns.
func (h *hub) peerLeft(id string) {
	h.mu.Lock()
	for _, seen := range h.announced {
		delete(seen, id)
	}
	var fns []func(string)
	for _, fn := range h.leaves {
		fns = append(fns, fn)
	}
	h.mu.Unlock()
	for _, fn := range fns {
		fn(id)
	}
}

// onPeerJoin registers a hook and replays the peers it has not heard of.
func (h *hub) onPeerJoin(hook string, fn func(string), connected []string) {
	h.mu.Lock()
	h.joins[hook] = fn
	if h.announced[hook] == nil {
		h.announced[hook] = map[string]bool{}
	}
	var replay []string
	for _, id := range connected {
		if !h.announced[hook][id] {
			h.announced[hook][id] = true
			replay = append(replay, id)
		}
	}
	h.mu.Unlock()
	for _, id := range replay {
		fn(id)
	}
}

func (h *hub) onPeerLeave(hook string, fn func(string)) {
	h.mu.Lock()
	h.leaves[hook] = fn
	h.mu.Unlock()
}

// shared returns the dispatcher of an action name and whether it is new.
func (h *hub) shared(name string) (*sharedAction, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if a := h.actions[name]; a != nil {
		return a, false
	}
	a := &sharedAction{receivers: map[*handle]func(json.RawMessage, string){}}
	h.actions[name] = a
	return a, true
}

func (a *sharedAction) dispatch(data json.RawMessage, peerID string) {
	a.mu.Lock()
	var fns []func(json.RawMessage, string)
	for _, fn := range a.receivers {
		fns = append(fns, fn)
	}
	a.mu.Unlock()
	for _, fn := range fns {
		fn(data, peerID)
	}
}

// handle is one caller's hold on a shared action.
type handle struct {
	shared *sharedAction
	send   func(payload any, target string) error
}

func (h *handle) Send(payload any, target string) error { return h.send(payload, target) }

func (h *handle) Connect(fn func(json.RawMessage, string)) {
	h.shared.mu.Lock()
	h.shared.receivers[h] = fn
	h.shared.mu.Unlock()
}

func (h *handle) Disconnect() {
	h.shared.mu.Lock()
	delete(h.shared.receivers, h)
	h.shared.mu.Unlock()
}

// TrysteroTransport carries a chat over a Trystero room.
type TrysteroTransport struct {
	room *trystero.Room
	hub  *hub
}

// NewTrysteroTransport joins roomID with cfg.
func NewTrysteroTransport(cfg trystero.Config, roomID string) (*TrysteroTransport, error) {
	room, err := trystero.Join(cfg, roomID)
	if err != nil {
		return nil, err
	}
	t := &TrysteroTransport{room: room, hub: newHub()}
	room.SetHandlers(t.hub.peerJoined, t.hub.peerLeft)
	return t, nil
}

// Room returns the underlying room.
func (t *TrysteroTransport) Room() *trystero.Room { return t.room }

func (t *TrysteroTransport) MakeAction(name string) ActionHandle {
	shared, fresh := t.hub.shared(name)
	if fresh {
		t.room.MakeAction(name, func(m trystero.Message) {
			switch m.Format {
			case trystero.FormatJSON:
				shared.dispatch(json.RawMessage(m.Data), m.PeerID)
			case trystero.FormatString:
				// Chitchatter sends objects; a string reaches a handler as
				// what it is, which no handler accepts.
				quoted, _ := json.Marshal(string(m.Data))
				shared.dispatch(quoted, m.PeerID)
			}
		})
	}
	return &handle{shared: shared, send: func(payload any, target string) error {
		data, err := marshalJSON(payload)
		if err != nil {
			return err
		}
		if target == "" {
			return t.room.Send(name, trystero.FormatJSON, data)
		}
		return t.room.Send(name, trystero.FormatJSON, data, target)
	}}
}

func (t *TrysteroTransport) OnPeerJoin(hook string, fn func(string)) {
	t.hub.onPeerJoin(hook, fn, t.room.Peers())
}

func (t *TrysteroTransport) OnPeerLeave(hook string, fn func(string)) { t.hub.onPeerLeave(hook, fn) }

func (t *TrysteroTransport) Peers() []string { return t.room.Peers() }

func (t *TrysteroTransport) Leave() { t.room.Leave() }

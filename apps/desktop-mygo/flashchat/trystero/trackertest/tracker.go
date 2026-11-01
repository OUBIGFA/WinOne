// Package trackertest is a minimal WebTorrent tracker for tests: it keeps
// one swarm per info_hash and passes offers and answers between its
// members as bittorrent-tracker's WebSocket server does, which is all the
// Trystero torrent strategy needs. It serves 127.0.0.1 only.
package trackertest

import (
	"encoding/json"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Server is a running tracker.
type Server struct {
	// URL is the tracker's ws:// address.
	URL string

	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
	swarms   map[string]map[string]*client
	// Interval is the announce interval it asks for, in seconds.
	Interval int
}

type client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *client) send(v any) {
	data, _ := json.Marshal(v)
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.WriteMessage(websocket.TextMessage, data)
}

// Start listens on a free port of 127.0.0.1.
func Start() (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{URL: "ws://" + l.Addr().String(), listener: l, swarms: map[string]map[string]*client{}, Interval: 10}
	s.server = &http.Server{Handler: http.HandlerFunc(s.serve)}
	go func() { _ = s.server.Serve(l) }()
	return s, nil
}

// Close stops the tracker and drops its clients.
func (s *Server) Close() { _ = s.server.Close() }

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type announce struct {
	Action   string            `json:"action"`
	InfoHash string            `json:"info_hash"`
	PeerID   string            `json:"peer_id"`
	NumWant  int               `json:"numwant"`
	Offers   []json.RawMessage `json:"offers"`
	Answer   json.RawMessage   `json:"answer"`
	OfferID  string            `json:"offer_id"`
	ToPeerID string            `json:"to_peer_id"`
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn}
	joined := map[string]string{} // info_hash → peer_id
	defer func() {
		s.mu.Lock()
		for hash, id := range joined {
			if s.swarms[hash][id] == c {
				delete(s.swarms[hash], id)
			}
		}
		s.mu.Unlock()
		_ = conn.Close()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var a announce
		if json.Unmarshal(data, &a) != nil || a.Action != "announce" || a.InfoHash == "" || a.PeerID == "" {
			c.send(map[string]any{"action": "announce", "failure reason": "invalid request"})
			continue
		}
		s.mu.Lock()
		swarm := s.swarms[a.InfoHash]
		if swarm == nil {
			swarm = map[string]*client{}
			s.swarms[a.InfoHash] = swarm
		}
		swarm[a.PeerID] = c
		joined[a.InfoHash] = a.PeerID
		var others []*client
		for id, other := range swarm {
			if id != a.PeerID {
				others = append(others, other)
			}
		}
		target := swarm[a.ToPeerID]
		count := len(swarm)
		s.mu.Unlock()
		if len(a.Answer) > 0 {
			if target != nil {
				target.send(map[string]any{"action": "announce", "answer": a.Answer, "offer_id": a.OfferID, "peer_id": a.PeerID, "info_hash": a.InfoHash})
			}
			continue
		}
		c.send(map[string]any{"action": "announce", "interval": s.Interval, "info_hash": a.InfoHash, "complete": 0, "incomplete": count})
		rand.Shuffle(len(others), func(i, j int) { others[i], others[j] = others[j], others[i] })
		for i, offer := range a.Offers {
			if i >= len(others) || (a.NumWant > 0 && i >= a.NumWant) {
				break
			}
			var o struct {
				OfferID string          `json:"offer_id"`
				Offer   json.RawMessage `json:"offer"`
			}
			if json.Unmarshal(offer, &o) != nil {
				continue
			}
			others[i].send(map[string]any{"action": "announce", "offer": o.Offer, "offer_id": o.OfferID, "peer_id": a.PeerID, "info_hash": a.InfoHash})
		}
	}
}

package trystero

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"winone/desktopmygo/flashchat/trystero/trackertest"
)

// events collects what a room's handlers heard, for tests to wait on.
type events struct {
	mu   sync.Mutex
	list []string
	wake chan struct{}
}

func newEvents() *events { return &events{wake: make(chan struct{}, 1)} }

func (e *events) add(s string) {
	e.mu.Lock()
	e.list = append(e.list, s)
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *events) wait(t *testing.T, want string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		e.mu.Lock()
		for _, s := range e.list {
			if s == want {
				e.mu.Unlock()
				return
			}
		}
		got := append([]string(nil), e.list...)
		e.mu.Unlock()
		select {
		case <-e.wake:
		case <-deadline:
			t.Fatalf("no %q within %v; got %q", want, within, got)
		}
	}
}

func (e *events) has(want string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.list {
		if s == want {
			return true
		}
	}
	return false
}

func localConfig(t *testing.T, tracker, password string) Config {
	rtc, err := ParseRTCConfig(`{"iceServers":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	return Config{AppID: "winone-test", Password: password, RelayURLs: []string{tracker}, RTC: rtc, Logf: t.Logf}
}

func joinWithEvents(t *testing.T, cfg Config, room string) (*Room, *events) {
	r, err := Join(cfg, room)
	if err != nil {
		t.Fatal(err)
	}
	e := newEvents()
	r.SetHandlers(func(id string) { e.add("join " + id) }, func(id string) { e.add("leave " + id) })
	return r, e
}

func TestTwoRoomsMeetExchangeAndLeave(t *testing.T) {
	if testing.Short() {
		t.Skip("opens WebRTC connections")
	}
	tracker, err := trackertest.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Close()
	a, ae := joinWithEvents(t, localConfig(t, tracker.URL, "secret"), "room")
	b, be := joinWithEvents(t, localConfig(t, tracker.URL, "secret"), "room")
	defer a.Leave()
	ae.wait(t, "join "+b.SelfID(), 30*time.Second)
	be.wait(t, "join "+a.SelfID(), 30*time.Second)
	if peers := a.Peers(); len(peers) != 1 || peers[0] != b.SelfID() {
		t.Fatalf("peers %v", peers)
	}

	got := make(chan Message, 4)
	b.MakeAction("g.0", func(m Message) { got <- m })
	// Inline, then chunked over the 16 KiB frame (a transcript backfill).
	if err := a.SendJSON("g.0", map[string]string{"text": "你好"}); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("历史", 20000)
	if err := a.Send("g.0", FormatString, []byte(big), b.SelfID()); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{`{"text":"你好"}`, big} {
		select {
		case m := <-got:
			if string(m.Data) != want || m.PeerID != a.SelfID() {
				t.Fatalf("message %d: %.40q from %s", i, m.Data, m.PeerID)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("message %d not received", i)
		}
	}

	// An action the receiver makes later still gets what came before.
	if err := b.Send("dm.8", FormatJSON, []byte(`{"isTyping":true}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	late := make(chan Message, 1)
	a.MakeAction("dm.8", func(m Message) { late <- m })
	select {
	case m := <-late:
		if !bytes.Equal(m.Data, []byte(`{"isTyping":true}`)) {
			t.Fatalf("late %s", m.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued message lost")
	}

	b.Leave()
	ae.wait(t, "leave "+b.SelfID(), 10*time.Second)
	if len(a.Peers()) != 0 {
		t.Fatalf("peers after leave %v", a.Peers())
	}
}

func TestAnotherPasswordNeverJoins(t *testing.T) {
	if testing.Short() {
		t.Skip("opens WebRTC connections")
	}
	tracker, err := trackertest.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Close()
	a, ae := joinWithEvents(t, localConfig(t, tracker.URL, "one"), "room")
	b, be := joinWithEvents(t, localConfig(t, tracker.URL, "two"), "room")
	defer a.Leave()
	defer b.Leave()
	time.Sleep(4 * time.Second)
	if ae.has("join "+b.SelfID()) || be.has("join "+a.SelfID()) {
		t.Fatal("peers with different passwords joined")
	}
}

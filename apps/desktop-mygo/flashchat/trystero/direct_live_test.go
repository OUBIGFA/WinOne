package trystero

import (
	"os"
	"testing"
	"time"
)

// Live, with WINONE_FLASHCHAT_LIVE=1: this computer learns its public
// address, which peers on other networks connect to, whether or not a
// proxy's TUN adapter is on, and two rooms meet on public trackers and
// exchange a message.
func TestLiveReachAndMeet(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_LIVE") == "" {
		t.Skip("set WINONE_FLASHCHAT_LIVE")
	}
	reach, err := ProbeReach(nil, 10*time.Second)
	t.Logf("reach %+v %v", reach, err)
	if err != nil || !reach.Public {
		t.Fatal("no public address")
	}
	cfg := Config{AppID: "winone-live-test", Password: genID(16)}
	room := "live-" + genID(12)
	a, err := Join(cfg, room)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Leave()
	b, err := Join(cfg, room)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Leave()
	got := make(chan string, 1)
	b.MakeAction("ping", func(m Message) { got <- string(m.Data) })
	a.MakeAction("ping", func(Message) {})
	deadline := time.Now().Add(90 * time.Second)
	for len(a.Peers()) == 0 || len(b.Peers()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("never met")
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("met after %v", 90*time.Second-time.Until(deadline))
	if err := a.Send("ping", FormatString, []byte("你好")); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != "你好" {
			t.Fatalf("got %q", m)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("message never arrived")
	}
}

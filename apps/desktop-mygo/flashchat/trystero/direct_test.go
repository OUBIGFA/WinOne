package trystero

import (
	"context"
	"net"
	"testing"

	"github.com/pion/transport/v5/stdnet"
)

func TestSocketsLeaveThroughTheirPhysicalAdapter(t *testing.T) {
	a := physicalAdapters{byIP: map[string]int{"192.168.31.123": 9, "fd00::5": 9}, primary4: 9, primary6: 9, tunnels: map[int]bool{7: true}}
	for _, c := range []struct {
		ip   string
		ipv6 bool
		want int
	}{{"192.168.31.123", false, 9}, {"", false, 9}, {"0.0.0.0", false, 9}, {"::", true, 9}, {"10.20.0.1", false, 0}, {"fd00::5", true, 9}} {
		if got := adapterFor(a, net.ParseIP(c.ip), c.ipv6); got != c.want {
			t.Errorf("adapterFor(%q) = %d, want %d", c.ip, got, c.want)
		}
	}
}

func TestTunnelAdaptersAreNotOffered(t *testing.T) {
	base, err := stdnet.NewNet()
	if err != nil {
		t.Fatal(err)
	}
	all, _ := base.Interfaces()
	if len(all) == 0 {
		t.Skip("no interfaces")
	}
	tunnel := all[0].Index
	n := &directNet{Net: base, adapters: func() (physicalAdapters, error) {
		return physicalAdapters{primary4: -1, tunnels: map[int]bool{tunnel: true}}, nil
	}}
	kept, _ := n.Interfaces()
	for _, ifc := range kept {
		if ifc.Index == tunnel {
			t.Fatal("a tunnel adapter is offered")
		}
	}
	if len(kept) != len(all)-1 {
		t.Fatalf("kept %d of %d", len(kept), len(all))
	}
	// Without a physical adapter to fall back on, everything stays.
	n.adapters = func() (physicalAdapters, error) { return physicalAdapters{tunnels: map[int]bool{tunnel: true}}, nil }
	if kept, _ := n.Interfaces(); len(kept) != len(all) {
		t.Fatal("the only route was dropped")
	}
}

// STUN names resolve through the room's resolver, not a TUN adapter's
// fake DNS, and are reused.
func TestServerNamesUseTheRealResolver(t *testing.T) {
	asked := 0
	n := &directNet{cache: map[string]resolved{}, resolve: func(_ context.Context, host string, ipv6 bool) ([]net.IP, error) {
		asked++
		if host != "stun.example" || ipv6 {
			t.Fatalf("asked %s %v", host, ipv6)
		}
		return []net.IP{net.ParseIP("203.0.113.7")}, nil
	}}
	for range 2 {
		addr, err := n.ResolveUDPAddr("udp4", "stun.example:3478")
		if err != nil || addr.String() != "203.0.113.7:3478" {
			t.Fatalf("%v %v", addr, err)
		}
	}
	if asked != 1 {
		t.Fatalf("asked %d times", asked)
	}
	if addr, err := n.ResolveUDPAddr("udp4", "198.51.100.1:19302"); err != nil || asked != 1 || addr.Port != 19302 {
		t.Fatal("a literal address is not looked up")
	}
}

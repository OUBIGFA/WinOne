package ai

import "testing"

func TestGatewayNetworkRequiresExplicitExposureAndExactOrigin(t *testing.T) {
	for _, c := range []struct {
		network            GatewayNetwork
		peer, host, origin string
		want               bool
	}{
		{GatewayNetwork{}, "127.0.0.1:4000", "localhost:3426", "", true},
		{GatewayNetwork{}, "192.168.1.2:4000", "127.0.0.1:3426", "", false},
		{GatewayNetwork{}, "127.0.0.1:4000", "attacker.invalid:3426", "", false},
		{GatewayNetwork{}, "127.0.0.1:4000", "localhost:3426", "https://chat.example", false},
		{GatewayNetwork{LAN: true, AllowIPs: []string{"192.168.1.0/24"}}, "192.168.1.2:4000", "192.168.1.10:3426", "", true},
		{GatewayNetwork{LAN: true, AllowIPs: []string{"192.168.1.0/24"}}, "10.0.0.2:4000", "192.168.1.10:3426", "", false},
		{GatewayNetwork{CORSOrigins: []string{"https://chat.example"}}, "127.0.0.1:4000", "localhost:3426", "https://chat.example", true},
		{GatewayNetwork{CORSOrigins: []string{"https://chat.example"}}, "127.0.0.1:4000", "localhost:3426", "https://chat.example.evil", false},
	} {
		if got := c.network.allowsPeer(c.peer, c.host) && c.network.allowsOrigin(c.origin); got != c.want {
			t.Fatalf("%+v got %v", c, got)
		}
	}
	for _, s := range []GatewayNetwork{{AllowIPs: []string{"192.168.*"}}, {CORSOrigins: []string{"*"}}, {CORSOrigins: []string{"https://*.example.com"}}, {CORSOrigins: []string{"https://example.com/path"}}, {CORSOrigins: []string{"https://user:pass@example.com"}}} {
		if s.validate() == nil {
			t.Fatalf("invalid rule accepted: %+v", s)
		}
	}
	for _, origin := range []string{"chrome-extension://abcdefgh", "moz-extension://123-abc", "safari-web-extension://123-abc"} {
		if err := (GatewayNetwork{CORSOrigins: []string{origin}}).validate(); err != nil {
			t.Fatalf("extension origin rejected: %v", err)
		}
	}
}

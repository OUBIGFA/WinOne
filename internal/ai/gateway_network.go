package ai

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

// GatewayNetwork is opt-in machine-local exposure. An empty allow list allows
// authenticated remote clients; entries restrict them to exact IPs or CIDRs.
type GatewayNetwork struct {
	LAN         bool     `json:"lan,omitempty"`
	AllowIPs    []string `json:"allowIPs,omitempty"`
	CORSOrigins []string `json:"corsOrigins,omitempty"`
}

func (s GatewayNetwork) validate() error {
	if len(s.AllowIPs) > 256 || len(s.CORSOrigins) > 256 {
		return errors.New("网关访问规则过多")
	}
	for _, rule := range s.AllowIPs {
		if _, err := netip.ParseAddr(rule); err != nil {
			if _, err = netip.ParsePrefix(rule); err != nil {
				return errors.New("允许的地址需为 IP 地址或 CIDR 网段")
			}
		}
	}
	for _, origin := range s.CORSOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.Hostname() == "" || strings.Contains(u.Host, "*") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !slices.Contains([]string{"http", "https", "chrome-extension", "moz-extension", "safari-web-extension"}, u.Scheme) {
			return errors.New("浏览器来源需为完整的 HTTP、HTTPS 或浏览器扩展来源，不带路径")
		}
	}
	return nil
}

func (s GatewayNetwork) allowsPeer(remote, host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddrPort(remote)
	if err != nil {
		return false
	}
	peer := ip.Addr().Unmap()
	localHost := strings.EqualFold(name, "localhost")
	if address, err := netip.ParseAddr(name); err == nil {
		localHost = address.Unmap().IsLoopback()
	} else if !localHost {
		return false
	}
	if !s.LAN {
		return peer.IsLoopback() && localHost
	}
	if peer.IsLoopback() || len(s.AllowIPs) == 0 {
		return true
	}
	for _, rule := range s.AllowIPs {
		if address, err := netip.ParseAddr(rule); err == nil && address.Unmap() == peer {
			return true
		}
		if prefix, err := netip.ParsePrefix(rule); err == nil && prefix.Contains(peer) {
			return true
		}
	}
	return false
}

func (s GatewayNetwork) allowsOrigin(origin string) bool {
	return origin == "" || slices.Contains(s.CORSOrigins, origin)
}

package trystero

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
)

// directNet is the network the room's peer connections use. A proxy's
// TUN adapter (Clash, sing-box, Karing...) takes the system's routes and
// its DNS: it answers STUN servers' names with fake addresses its TUN
// alone understands, and may drop UDP it cannot forward. Peers on other
// networks then never learn an address of this computer, and cannot
// join. directNet keeps WebRTC on the physical network:
//
//   - STUN and TURN names are resolved over DNS-over-HTTPS (TCP, which
//     proxies carry), so they are the servers' real addresses;
//   - UDP sockets leave through the physical adapter that owns their
//     address, or the default physical one (IP_UNICAST_IF), whatever
//     routes a TUN adapter added;
//   - the TUN adapter's own address is not offered as a candidate.
//
// Without a TUN adapter it behaves as the standard network.
type directNet struct {
	*stdnet.Net
	adapters adapterSource
	resolve  func(ctx context.Context, host string, ipv6 bool) ([]net.IP, error)

	mu    sync.Mutex
	cache map[string]resolved
}

type resolved struct {
	ips []net.IP
	at  time.Time
}

// physicalAdapters describes this computer's adapters: the indexes of
// the physical ones that reach a gateway, by address, the default one,
// and the indexes of tunnel adapters to leave out.
type physicalAdapters struct {
	byIP     map[string]int
	primary4 int
	primary6 int
	tunnels  map[int]bool
}

type adapterSource func() (physicalAdapters, error)

// newDirectNet makes the network for a room.
func newDirectNet() (transport.Net, error) {
	base, err := stdnet.NewNet()
	if err != nil {
		return nil, err
	}
	return &directNet{Net: base, adapters: readAdapters, resolve: resolveHost, cache: map[string]resolved{}}, nil
}

// Interfaces leaves out tunnel adapters while a physical one reaches a
// gateway: a candidate on a TUN address only reaches the proxy.
func (n *directNet) Interfaces() ([]*transport.Interface, error) {
	_ = n.Net.UpdateInterfaces()
	all, err := n.Net.Interfaces()
	if err != nil {
		return nil, err
	}
	a, err := n.adapters()
	if err != nil || a.primary4 == 0 && a.primary6 == 0 {
		return all, nil
	}
	kept := all[:0:0]
	for _, ifc := range all {
		if !a.tunnels[ifc.Index] {
			kept = append(kept, ifc)
		}
	}
	return kept, nil
}

// adapterFor is the physical adapter a socket at ip leaves through: the
// one that owns it, or the default one for its family; 0 leaves routing
// to the system.
func adapterFor(a physicalAdapters, ip net.IP, ipv6 bool) int {
	if ip != nil && !ip.IsUnspecified() {
		if index, ok := a.byIP[ip.String()]; ok {
			return index
		}
		return 0
	}
	if ipv6 {
		return a.primary6
	}
	return a.primary4
}

func (n *directNet) listenConfig(network string, ip net.IP) *net.ListenConfig {
	a, err := n.adapters()
	if err != nil {
		return &net.ListenConfig{}
	}
	ipv6 := network == "udp6" || ip != nil && ip.To4() == nil && !ip.IsUnspecified()
	index := adapterFor(a, ip, ipv6)
	return &net.ListenConfig{Control: unicastInterface(index, ipv6)}
}

func (n *directNet) ListenUDP(network string, laddr *net.UDPAddr) (transport.UDPConn, error) {
	var ip net.IP
	address := ""
	if laddr != nil {
		ip, address = laddr.IP, laddr.String()
	}
	conn, err := n.listenConfig(network, ip).ListenPacket(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	udp, ok := conn.(*net.UDPConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("not a UDP connection")
	}
	return udp, nil
}

func (n *directNet) ListenPacket(network, address string) (net.PacketConn, error) {
	if network != "udp" && network != "udp4" && network != "udp6" {
		return n.Net.ListenPacket(network, address)
	}
	var ip net.IP
	if host, _, err := net.SplitHostPort(address); err == nil {
		ip = net.ParseIP(host)
	}
	return n.listenConfig(network, ip).ListenPacket(context.Background(), network, address)
}

// ResolveUDPAddr resolves STUN and TURN servers to their real addresses.
func (n *directNet) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(host) != nil {
		return net.ResolveUDPAddr(network, address)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	ipv6 := network == "udp6"
	ips, err := n.lookup(host, ipv6)
	if err != nil {
		return nil, err
	}
	return &net.UDPAddr{IP: ips[0], Port: port}, nil
}

// lookupTTL is how long a name stays resolved.
const lookupTTL = 10 * time.Minute

func (n *directNet) lookup(host string, ipv6 bool) ([]net.IP, error) {
	key := host + "|" + strconv.FormatBool(ipv6)
	n.mu.Lock()
	if r, ok := n.cache[key]; ok && time.Since(r.at) < lookupTTL {
		n.mu.Unlock()
		return r.ips, nil
	}
	n.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	ips, err := n.resolve(ctx, host, ipv6)
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	n.cache[key] = resolved{ips: ips, at: time.Now()}
	n.mu.Unlock()
	return ips, nil
}

// dohServers answer JSON DNS queries over HTTPS. Their certificates name
// their IP addresses, so reaching them needs no DNS of its own.
var dohServers = []string{
	"https://223.5.5.5/resolve",
	"https://1.1.1.1/dns-query",
	"https://8.8.8.8/resolve",
}

var dohClient = &http.Client{Timeout: 5 * time.Second}

// resolveHost asks the DoH servers at once and takes the first answer; the
// system resolver, which a TUN adapter may answer with fake addresses,
// is the last resort.
func resolveHost(ctx context.Context, host string, ipv6 bool) ([]net.IP, error) {
	kind := 1
	if ipv6 {
		kind = 28
	}
	type answer struct {
		ips []net.IP
		err error
	}
	answers := make(chan answer, len(dohServers))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, server := range dohServers {
		go func(server string) {
			ips, err := queryDoH(ctx, server, host, kind)
			answers <- answer{ips, err}
		}(server)
	}
	var errs []error
	for range dohServers {
		a := <-answers
		if a.err == nil && len(a.ips) > 0 {
			return a.ips, nil
		}
		errs = append(errs, a.err)
	}
	family := "ip4"
	if ipv6 {
		family = "ip6"
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, family, host)
	if err == nil && len(ips) > 0 {
		return ips, nil
	}
	return nil, fmt.Errorf("resolve %s: %w", host, errors.Join(append(errs, err)...))
}

func queryDoH(ctx context.Context, server, host string, kind int) ([]net.IP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"?name="+host+"&type="+strconv.Itoa(kind), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := dohClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", server, resp.Status)
	}
	var reply struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply); err != nil {
		return nil, err
	}
	var ips []net.IP
	for _, a := range reply.Answer {
		if ip := net.ParseIP(a.Data); a.Type == kind && ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%s: no address for %s", server, host)
	}
	return ips, nil
}

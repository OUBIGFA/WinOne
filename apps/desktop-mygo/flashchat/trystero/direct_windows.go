//go:build windows

package trystero

import (
	"encoding/binary"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Adapter types (IANA ifType) of physical networks a peer can be reached
// on: Ethernet, Wi-Fi and mobile broadband.
var physicalTypes = map[uint32]bool{6: true, 71: true, 243: true, 244: true}

// Adapter types of tunnels: virtual (Wintun, TAP) and tunnel interfaces.
var tunnelTypes = map[uint32]bool{53: true, 131: true}

var adapters struct {
	sync.Mutex
	value physicalAdapters
	err   error
	at    time.Time
}

// readAdapters reads the adapters at most once a few seconds: every
// socket of a connection asks.
func readAdapters() (physicalAdapters, error) {
	adapters.Lock()
	defer adapters.Unlock()
	if !adapters.at.IsZero() && time.Since(adapters.at) < 5*time.Second {
		return adapters.value, adapters.err
	}
	adapters.value, adapters.err = queryAdapters()
	adapters.at = time.Now()
	return adapters.value, adapters.err
}

func queryAdapters() (physicalAdapters, error) {
	size := uint32(16 << 10)
	var buf []byte
	for range 4 {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_ANYCAST,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return physicalAdapters{}, err
		}
		buf = nil
	}
	if buf == nil {
		return physicalAdapters{}, windows.ERROR_BUFFER_OVERFLOW
	}
	a := physicalAdapters{byIP: map[string]int{}, tunnels: map[int]bool{}}
	var metric4, metric6 uint32
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		if tunnelTypes[aa.IfType] {
			a.tunnels[int(aa.IfIndex)] = true
			if aa.Ipv6IfIndex != 0 {
				a.tunnels[int(aa.Ipv6IfIndex)] = true
			}
			continue
		}
		if !physicalTypes[aa.IfType] || aa.OperStatus != windows.IfOperStatusUp || aa.FirstGatewayAddress == nil {
			continue
		}
		for u := aa.FirstUnicastAddress; u != nil; u = u.Next {
			ip := u.Address.IP()
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				a.byIP[ip.String()] = int(aa.IfIndex)
			} else {
				a.byIP[ip.String()] = int(aa.Ipv6IfIndex)
			}
		}
		if aa.IfIndex != 0 && (a.primary4 == 0 || aa.Ipv4Metric < metric4) {
			a.primary4, metric4 = int(aa.IfIndex), aa.Ipv4Metric
		}
		if aa.Ipv6IfIndex != 0 && (a.primary6 == 0 || aa.Ipv6Metric < metric6) {
			a.primary6, metric6 = int(aa.Ipv6IfIndex), aa.Ipv6Metric
		}
	}
	return a, nil
}

// ipUnicastIf is IP_UNICAST_IF and IPV6_UNICAST_IF.
const ipUnicastIf = 31

// unicastInterface sends a socket's datagrams out of adapter index,
// whatever the routes say; 0 leaves the socket alone.
func unicastInterface(index int, ipv6 bool) func(string, string, syscall.RawConn) error {
	if index == 0 {
		return nil
	}
	return func(network, _ string, c syscall.RawConn) error {
		var setErr error
		err := c.Control(func(fd uintptr) {
			if ipv6 || network == "udp6" {
				setErr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, ipUnicastIf, index)
				return
			}
			// IPv4 takes the index in network byte order.
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], uint32(index))
			setErr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, int(binary.LittleEndian.Uint32(b[:])))
		})
		if err != nil {
			return err
		}
		return setErr
	}
}

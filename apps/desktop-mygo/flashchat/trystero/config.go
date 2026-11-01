package trystero

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DefaultRelayURLs are the WebTorrent trackers of @trystero-p2p/torrent
// 0.26.0, in its order; a room without its own uses the first Redundancy.
var DefaultRelayURLs = []string{
	"wss://open.ftorrent.com",
	"wss://tracker.webtorrent.dev",
	"wss://tracker.openwebtorrent.com",
	"wss://tracker.btorrent.xyz",
	"wss://tracker.files.fm:7073/announce",
}

// defaultRedundancy is the torrent strategy's number of default trackers.
const defaultRedundancy = 3

// Config is what a room is joined with: Trystero's join config, as far as
// the torrent strategy uses it.
type Config struct {
	AppID string
	// Password encrypts signalling and is proved in a handshake; peers with
	// another password never connect.
	Password string
	// RelayURLs are the trackers; empty uses DefaultRelayURLs[:Redundancy].
	RelayURLs  []string
	Redundancy int
	// RTC overrides the peer connections' configuration, as rtcConfig.
	RTC *RTCConfig
	// Logf receives warnings, as Trystero's console.warn; nil drops them.
	Logf func(format string, args ...any)
}

// relays returns the trackers to use, as utils.ts's getRelays.
func (c Config) relays() []string {
	if len(c.RelayURLs) > 0 {
		return c.RelayURLs
	}
	n := c.Redundancy
	if n <= 0 {
		n = defaultRedundancy
	}
	return DefaultRelayURLs[:min(n, len(DefaultRelayURLs))]
}

// ICEServer is an RTCIceServer.
type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// RTCConfig is the part of an RTCConfiguration that applies to data
// channels: ICE servers and the transport policy.
type RTCConfig struct {
	// ICEServers replaces the default STUN servers when set (even empty).
	ICEServers    []ICEServer
	HasICEServers bool
	// ICETransportPolicy is "all" or "relay".
	ICETransportPolicy string
}

// ParseRTCConfig reads an RTCConfiguration written as JSON, as the flash
// chat's network settings take it: an object, whose iceServers entries
// name urls as a string or a list.
func ParseRTCConfig(text string) (*RTCConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil || raw == nil {
		return nil, errors.New("RTC configuration must be a JSON object")
	}
	var c RTCConfig
	if servers, ok := raw["iceServers"]; ok {
		c.HasICEServers = true
		var list []struct {
			URLs       json.RawMessage `json:"urls"`
			Username   any             `json:"username"`
			Credential any             `json:"credential"`
		}
		if err := json.Unmarshal(servers, &list); err != nil {
			return nil, errors.New("iceServers must be a list")
		}
		for i, s := range list {
			var server ICEServer
			var one string
			if json.Unmarshal(s.URLs, &one) == nil {
				server.URLs = []string{one}
			} else if json.Unmarshal(s.URLs, &server.URLs) != nil || len(server.URLs) == 0 {
				return nil, fmt.Errorf("iceServers[%d].urls must be a URL or a list of URLs", i)
			}
			if s.Username != nil {
				name, ok := s.Username.(string)
				if !ok {
					return nil, fmt.Errorf("iceServers[%d].username must be text", i)
				}
				server.Username = name
			}
			if s.Credential != nil {
				credential, ok := s.Credential.(string)
				if !ok {
					return nil, fmt.Errorf("iceServers[%d].credential must be text", i)
				}
				server.Credential = credential
			}
			c.ICEServers = append(c.ICEServers, server)
		}
	}
	if policy, ok := raw["iceTransportPolicy"]; ok {
		if json.Unmarshal(policy, &c.ICETransportPolicy) != nil || (c.ICETransportPolicy != "all" && c.ICETransportPolicy != "relay") {
			return nil, errors.New(`iceTransportPolicy must be "all" or "relay"`)
		}
	}
	return &c, nil
}

// iceServers returns the servers a peer connection uses.
func (c *RTCConfig) iceServers() []ICEServer {
	if c != nil && c.HasICEServers {
		return c.ICEServers
	}
	return DefaultICEServers
}

// ValidRelayURL reports whether a tracker address is one the flash chat
// accepts: a secure WebSocket, or a plain one on this computer for tests
// and local services.
func ValidRelayURL(text string) bool {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "wss" || (u.Scheme == "ws" && u.Hostname() == "127.0.0.1")
}

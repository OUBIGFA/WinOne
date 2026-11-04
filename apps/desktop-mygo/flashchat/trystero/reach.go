package trystero

import (
	"io"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
)

// Reach is what this computer can offer peers on other networks: an
// address the STUN servers saw (a "server reflexive" candidate) or a TURN
// relay. Without either, only peers on the same network can connect.
type Reach struct {
	Public, Relayed bool
}

// ProbeReach gathers this computer's connection candidates with the room's
// ICE servers, as a peer connection would, for at most timeout.
func ProbeReach(rtc *RTCConfig, timeout time.Duration) (Reach, error) {
	var config webrtc.Configuration
	for _, s := range rtc.iceServers() {
		config.ICEServers = append(config.ICEServers, webrtc.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential})
	}
	var se webrtc.SettingEngine
	se.LoggerFactory = &logging.DefaultLoggerFactory{Writer: io.Discard}
	if dn, err := newDirectNet(); err == nil {
		se.SetNet(dn)
	}
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(config)
	if err != nil {
		return Reach{}, err
	}
	defer pc.Close()
	found := make(chan webrtc.ICECandidateType, 16)
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			select {
			case found <- c.Typ:
			default:
			}
		}
	})
	if _, err := pc.CreateDataChannel("probe", nil); err != nil {
		return Reach{}, err
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return Reach{}, err
	}
	done := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		return Reach{}, err
	}
	var reach Reach
	deadline := time.After(timeout)
	for {
		select {
		case typ := <-found:
			switch typ {
			case webrtc.ICECandidateTypeSrflx, webrtc.ICECandidateTypePrflx:
				reach.Public = true
			case webrtc.ICECandidateTypeRelay:
				reach.Relayed = true
			}
		case <-done:
			// Candidates found before completion are already queued.
			for {
				select {
				case typ := <-found:
					reach.Public = reach.Public || typ == webrtc.ICECandidateTypeSrflx || typ == webrtc.ICECandidateTypePrflx
					reach.Relayed = reach.Relayed || typ == webrtc.ICECandidateTypeRelay
				default:
					return reach, nil
				}
			}
		case <-deadline:
			return reach, nil
		}
	}
}

module winone/desktopmygo

go 1.27.3

require (
	github.com/ebitengine/purego v0.11.1
	github.com/egoist/mygo v0.8.3
	github.com/gorilla/websocket v1.5.3
	github.com/pion/logging v0.2.4
	github.com/pion/transport/v5 v5.1.4
	github.com/pion/webrtc/v4 v4.2.26
	golang.org/x/image v0.46.0
	golang.org/x/net v0.59.0
	golang.org/x/sys v0.48.0
	golang.org/x/text v0.43.0
	winone v0.0.0-00010101000000-000000000000
	winone/modules/nativeui v0.0.0
	winone/resultui v0.0.0-00010101000000-000000000000
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pion/datachannel v1.6.3 // indirect
	github.com/pion/dtls/v3 v3.1.15 // indirect
	github.com/pion/ice/v4 v4.4.8 // indirect
	github.com/pion/interceptor v0.1.51 // indirect
	github.com/pion/mdns/v2 v2.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.20 // indirect
	github.com/pion/rtp v1.10.6 // indirect
	github.com/pion/sctp v1.12.0 // indirect
	github.com/pion/sdp/v3 v3.0.20 // indirect
	github.com/pion/srtp/v3 v3.1.3 // indirect
	github.com/pion/stun/v4 v4.0.1 // indirect
	github.com/pion/turn/v5 v5.1.3 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/time v0.14.0 // indirect
)

replace winone => ../..

replace winone/modules/wintray => ../../modules/wintray

replace fyne.io/systray => ../../third_party/systray

replace winone/resultui => ../result-ui

replace winone/modules/nativeui => ../../modules/nativeui

// Prepared from verified upstream by build/prepare-mygo-filedrop.ps1.
replace github.com/egoist/mygo => ../../dist/resources/sources/mygo-winone-v0.8.3

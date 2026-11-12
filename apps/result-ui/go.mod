module winone/resultui

go 1.27.3

require (
	github.com/egoist/mygo v0.9.0
	github.com/gorilla/websocket v1.5.3
	golang.org/x/sys v0.48.0
	winone v0.0.0-00010101000000-000000000000
	winone/modules/nativeui v0.0.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace winone => ../..

replace winone/modules/wintray => ../../modules/wintray

replace fyne.io/systray => ../../third_party/systray

replace winone/modules/nativeui => ../../modules/nativeui

// Prepared from verified upstream by build/prepare-mygo-filedrop.ps1.
replace github.com/egoist/mygo => ../../dist/resources/sources/mygo-winone-v0.9.0

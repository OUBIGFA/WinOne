module winone/desktopmygo

go 1.27.3

require (
	github.com/egoist/mygo v0.9.0
	github.com/go-ole/go-ole v1.3.0
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/sys v0.48.0
	winone v0.0.0-00010101000000-000000000000
	winone/modules/nativeui v0.0.0
	winone/resultui v0.0.0-00010101000000-000000000000
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace winone => ../..

replace winone/modules/wintray => ../../modules/wintray

replace fyne.io/systray => ../../third_party/systray

replace winone/resultui => ../result-ui

replace winone/modules/nativeui => ../../modules/nativeui

// Prepared from verified upstream by build/prepare-mygo-filedrop.ps1.
replace github.com/egoist/mygo => ../../dist/resources/sources/mygo-winone-v0.9.0

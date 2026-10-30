module winone/desktopmygo

go 1.27.3

require (
	github.com/egoist/mygo v0.7.3
	golang.org/x/sys v0.48.0
	winone v0.0.0-00010101000000-000000000000
	winone/resultui v0.0.0-00010101000000-000000000000
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace winone => ../..

replace winone/modules/wintray => ../../modules/wintray

replace fyne.io/systray => ../../third_party/systray

replace winone/resultui => ../result-ui

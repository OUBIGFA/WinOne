module winone/resultui

go 1.27.1

require (
	github.com/egoist/mygo v0.2.13-0.20261006044557-3ad015a7f887
	github.com/gorilla/websocket v1.5.3
	github.com/yuin/goldmark v1.8.6
	golang.org/x/sys v0.48.0
	winone v0.0.0-00010101000000-000000000000
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/fyne-io/image v0.1.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	golang.org/x/image v0.46.0 // indirect
)

replace winone => ../..

replace winone/modules/wintray => ../../modules/wintray

replace fyne.io/systray => ../../third_party/systray

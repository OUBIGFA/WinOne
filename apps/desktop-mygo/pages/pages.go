// Package pages registers every page of the window with the shell. Each
// page lives in its own package and registers itself from init.
package pages

import (
	_ "winone/desktopmygo/pages/aiservices"
	_ "winone/desktopmygo/pages/capture"
	_ "winone/desktopmygo/pages/convenience"
	_ "winone/desktopmygo/pages/interconnect"
	_ "winone/desktopmygo/pages/overview"
	_ "winone/desktopmygo/pages/selection"
	_ "winone/desktopmygo/pages/settings"
	_ "winone/desktopmygo/pages/wintray"
)

// Package pages registers every page of the window with the shell. Each
// page lives in its own package and registers itself from init.
package pages

import (
	_ "winone/desktopmygo/pages/overview"
)

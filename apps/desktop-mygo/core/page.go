package core

import "github.com/egoist/mygo/ui"

// Page is one page of the window: a module's settings, a tool, Chat.
type Page interface {
	// View builds the page inside the shell's content frame.
	View(c *ui.Context)
}

// PageFunc adapts a view function to Page.
type PageFunc func(c *ui.Context)

// View implements Page.
func (f PageFunc) View(c *ui.Context) { f(c) }

// Layout says how the shell frames a page.
type Layout int

const (
	// Settings pages scroll in a centered column, with the save rail.
	Settings Layout = iota
	// Workspace pages scroll in a wider column, with the save rail.
	Workspace
	// Conversation pages fill the frame and manage their own scrolling;
	// they show no save rail.
	Conversation
)

// Closer is a page that releases something when the shell leaves it, as a
// check the Host runs for it.
type Closer interface{ Close() }

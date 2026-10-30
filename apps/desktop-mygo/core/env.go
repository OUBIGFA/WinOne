// Package core is what every page of the desktop window shares beyond the
// visual kit: the Host client, the UI thread, the shell's navigation and
// save rail, and helpers for drafts that must keep fields this window does
// not edit.
package core

import (
	"winone/desktopmygo/host"

	internalhost "winone/internal/host"
)

// Env is the environment the shell gives each page. Fields are set once
// when the window starts; functions are safe to call on the UI thread
// only, unless documented otherwise.
type Env struct {
	// Host reaches the WinOne Host. Its calls block: call them from Go.
	Host *host.Client
	// Post runs fn on the UI thread and draws a new frame. It is safe from
	// any goroutine; after the window closes it drops fn.
	Post func(fn func())
	// Snapshot returns the last Host snapshot the shell read.
	Snapshot func() internalhost.Snapshot
	// Refresh asks the shell to read the snapshot again, as after a
	// module was enabled or a setting saved elsewhere.
	Refresh func()
	// Navigate asks the shell to show another page through the
	// unsaved-change guard; arrive runs once the page shows.
	Navigate func(page string, arrive func())
	// RequestChange applies a change that hides or replaces a section of
	// the current page (switching tabs) through the same guard.
	RequestChange func(apply func())
	// Activity collects the drafts and running operations of the page.
	Activity *Activity
	// Rail holds the page's Save and Undo for the shell's action rail.
	Rail *Rail
	// Dialogs runs native dialogs owned by the window.
	Dialogs Dialogs
	// Spawn starts background work; nil starts a goroutine. Tests set it
	// to know when every request finished.
	Spawn func(func())
	// Directory is the profile's data directory, where the Host keeps its
	// settings and logs. The shell sets it.
	Directory string
	// SidebarStartup returns how the sidebar opens: "collapsed" (icons
	// only) or "expanded" (icons and names).
	SidebarStartup func() string
	// SetSidebarStartup stores how the sidebar opens and folds or unfolds
	// it at once, as the setting shows its effect when chosen.
	SetSidebarStartup func(value string) error
}

func (env *Env) spawn(fn func()) {
	if env.Spawn != nil {
		env.Spawn(fn)
		return
	}
	go fn()
}

// Dialogs are the window's native pickers. They block until the user
// answers: call them from Go, never on the UI thread.
type Dialogs interface {
	// PickFile returns a file path, or "" when cancelled. filter is a
	// display name and its extensions, empty for all files.
	PickFile(title string, filter Filter) (string, error)
	// PickFiles returns several files, or nil when cancelled.
	PickFiles(title string, filter Filter) ([]string, error)
	// PickFolder returns a folder, or "" when cancelled.
	PickFolder(title string) (string, error)
	// PickFolders returns several folders, or nil when cancelled.
	PickFolders(title string) ([]string, error)
	// SaveFile returns where to save, or "" when cancelled.
	SaveFile(title string, filter Filter, defaultName string) (string, error)
}

// Filter names the files a picker offers.
type Filter struct {
	Name       string
	Extensions []string
}

// Go runs work off the UI thread and then done on it with the result.
func Go[T any](env *Env, work func() (T, error), done func(T, error)) {
	env.spawn(func() {
		value, err := work()
		env.Post(func() { done(value, err) })
	})
}

// Do is Go for work without a result.
func Do(env *Env, work func() error, done func(error)) {
	env.spawn(func() {
		err := work()
		env.Post(func() { done(err) })
	})
}

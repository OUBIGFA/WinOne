// Package shell is the frame of the WinOne window: the title bar, the
// sidebar of pages, the content area with its save rail, the guard that
// protects unsaved changes, and reading the Host's snapshot that decides
// theme, language and the first page.
package shell

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	internalhost "winone/internal/host"
)

// WindowPort is the native window the shell draws in. Its methods run on
// the UI thread.
type WindowPort interface {
	Minimize()
	ToggleMaximize()
	IsMaximized() bool
	// Close closes as the title bar's button does: through the guard,
	// then hiding or exiting as the settings say.
	Close()
	// ApproveClose closes after the user chose to discard changes.
	ApproveClose()
	// CancelClose forgets a close the user declined.
	CancelClose()
	// ApplySettings applies the snapshot's theme and whether closing
	// keeps the window in the background.
	ApplySettings(theme string, keepRunning bool)
}

// Options configure a Shell.
type Options struct {
	// Directory is the profile, where the window keeps its own state.
	Directory string
	// EnsureHost starts the Host when needed and returns its snapshot.
	// It blocks; the shell calls it off the UI thread.
	EnsureHost func() (internalhost.Snapshot, error)
	// StartupWarning is shown when a remembered administrator start
	// could not be used.
	StartupWarning string
	// OpenSettings asks for the default page once the snapshot arrives.
	OpenSettings bool
}

// Shell is the window's frame and navigation.
type Shell struct {
	env    *core.Env
	window WindowPort
	opts   Options
	ui     uiState

	snapshot internalhost.Snapshot
	loaded   bool
	loading  bool
	loadErr  string

	page     string
	instance core.Page
	// arrived pages, recreated on each visit: as the React window
	// unmounted a page, leaving one ends its drafts and reads.
	initial bool

	pending *pending
	closing bool

	sidebarWidth float32
	scroll       ui.ScrollState
	railHeight   float32
	// toasts wait for the next frame, which shows them.
	toasts []string
	// intents are prefills arrived at their page, until it takes them;
	// reading tells a read of the Host's queues in progress.
	intents   map[string][]core.Intent
	reading   bool
	intentErr string
	focuses   int
}

type pendingKind int

const (
	pendingPage pendingKind = iota
	pendingChange
	pendingClose
)

type pending struct {
	kind   pendingKind
	page   string
	arrive func()
	apply  func()
	err    string
}

// New makes the shell; call Start once the window exists.
func New(env *core.Env, window WindowPort, opts Options) *Shell {
	s := &Shell{env: env, window: window, opts: opts, page: "overview", ui: loadUIState(opts.Directory)}
	s.sidebarWidth = collapsedWidth
	if s.ui.SidebarStartup == "expanded" {
		s.sidebarWidth = s.ui.ExpandedWidth
	}
	env.Snapshot = func() internalhost.Snapshot { return s.snapshot }
	env.Refresh = s.refresh
	env.Navigate = s.requestPage
	env.RequestChange = s.requestChange
	env.Directory = opts.Directory
	env.SidebarStartup = func() string { return s.ui.SidebarStartup }
	env.SetSidebarStartup = s.setSidebarStartup
	env.Focuses = func() int { return s.focuses }
	env.TakeIntents = func(page string) []core.Intent {
		intents := s.intents[page]
		delete(s.intents, page)
		return intents
	}
	env.Toast = func(message string) {
		s.toasts = append(s.toasts, message)
		env.Post(func() {})
	}
	if env.Activity == nil {
		env.Activity = &core.Activity{}
	}
	if env.Rail == nil {
		env.Rail = &core.Rail{}
	}
	return s
}

// Start reads the snapshot for the first time.
func (s *Shell) Start() { s.refresh() }

func (s *Shell) refresh() {
	if s.loading {
		return
	}
	s.loading = true
	core.Go(s.env, s.opts.EnsureHost, func(snapshot internalhost.Snapshot, err error) {
		s.loading = false
		if err != nil {
			s.loadErr = err.Error()
			return
		}
		s.loadErr = ""
		s.apply(snapshot)
	})
}

func (s *Shell) apply(snapshot internalhost.Snapshot) {
	first := !s.loaded
	s.snapshot, s.loaded = snapshot, true
	settings := snapshot.Settings
	i18n.SetLocale(i18n.Resolve(settings.Language, snapshot.Locale))
	keep := true
	if settings.Startup != nil && settings.Startup.KeepUIRunning != nil {
		keep = *settings.Startup.KeepUIRunning
	}
	s.window.ApplySettings(settings.Theme, keep)
	if first && !s.initial {
		s.initial = true
		s.requestPage(s.defaultPage(), nil)
	}
	if first {
		s.ReadIntents()
	}
}

// intentPages are the Host's prefill queues and the pages that show them.
var intentPages = []struct{ method, page string }{
	{"interconnect.pending", "interconnect"},
	{"directory-links.pending", "convenience"},
}

// Focused tells that the window gained the focus: prefills are read again
// and pages see the new focus count.
func (s *Shell) Focused() {
	s.focuses++
	s.ReadIntents()
}

// ReadIntents takes the prefills other launches queued in the Host and
// shows their pages, through the unsaved-change guard: a declined change
// drops them, so none waits to open on a later visit. The window reads
// them when it starts, when another launch wakes it and when it gains the
// focus.
func (s *Shell) ReadIntents() {
	if !s.loaded || s.reading {
		return
	}
	s.reading = true
	type found struct {
		page   string
		intent core.Intent
	}
	core.Go(s.env, func() ([]found, error) {
		var all []found
		var failure error
		for _, queue := range intentPages {
			var intent core.Intent
			if err := s.env.Host.CallCompletingInto(queue.method, nil, &intent); err != nil {
				failure = err
				continue
			}
			if intent.Requested {
				all = append(all, found{queue.page, intent})
			}
		}
		return all, failure
	}, func(all []found, err error) {
		s.reading = false
		s.intentErr = ""
		if err != nil {
			s.intentErr = err.Error()
		}
		for _, f := range all {
			f := f
			s.requestPage(f.page, func() {
				if s.intents == nil {
					s.intents = map[string][]core.Intent{}
				}
				s.intents[f.page] = append(s.intents[f.page], f.intent)
			})
		}
	})
}

// pendingIntentPage is a page with prefills it has not taken yet, which a
// request for the settings shows instead of the default page.
func (s *Shell) pendingIntentPage() string {
	for _, queue := range intentPages {
		if len(s.intents[queue.page]) > 0 {
			return queue.page
		}
	}
	return ""
}

// defaultPage is the page the settings open on, if it still exists.
func (s *Shell) defaultPage() string {
	page := s.snapshot.Settings.DefaultPage
	if slices.ContainsFunc(pages(), func(p pageSpec) bool { return p.id == page }) {
		return page
	}
	return "overview"
}

// Wake handles another launch asking for the window; settings is true for
// --open-settings, which shows the default page through the guard.
func (s *Shell) Wake(settings bool) {
	if settings && s.loaded {
		page := s.pendingIntentPage()
		if page == "" {
			page = s.defaultPage()
		}
		s.requestPage(page, nil)
	}
	s.ReadIntents()
}

// requestPage shows target, after the user decides about unsaved changes.
func (s *Shell) requestPage(target string, arrive func()) {
	if target == s.page && s.instance != nil {
		if arrive != nil {
			arrive()
		}
		return
	}
	if dirty, busy := s.env.Activity.State(); dirty || busy {
		s.pending = &pending{kind: pendingPage, page: target, arrive: arrive}
		return
	}
	s.navigate(target, arrive)
}

func (s *Shell) navigate(target string, arrive func()) {
	s.env.Activity.Reset()
	s.closePage()
	s.page = target
	s.scroll = ui.ScrollState{}
	if arrive != nil {
		arrive()
	}
}

// requestChange applies a tab switch through the same decision.
func (s *Shell) requestChange(apply func()) {
	if dirty, busy := s.env.Activity.State(); !dirty && !busy {
		apply()
		return
	}
	s.pending = &pending{kind: pendingChange, apply: apply}
}

// RequestClose is the window's close of a page with activity: it asks the
// user, and reports whether the window may close at once.
func (s *Shell) RequestClose() bool {
	dirty, busy := s.env.Activity.State()
	if !dirty && !busy {
		return true
	}
	s.pending = &pending{kind: pendingClose}
	return false
}

// Activity reports the page's unsaved changes and running work.
func (s *Shell) Activity() (dirty, busy bool) { return s.env.Activity.State() }

// Discarded resets every page after the user discarded changes and the
// window stayed in the background, so it reopens without the old draft.
func (s *Shell) Discarded() {
	s.env.Activity.Reset()
	s.closePage()
}

// closePage ends the current page's instance; the next visit makes a new
// one, as the React window remounted a page.
func (s *Shell) closePage() {
	if closer, ok := s.instance.(core.Closer); ok {
		closer.Close()
	}
	s.instance = nil
}

// settle finishes a pending decision once the work it waited for ended,
// as a save that completed while the dialog was open.
func (s *Shell) settle() {
	if s.pending == nil {
		return
	}
	if dirty, busy := s.env.Activity.State(); !dirty && !busy {
		s.finish()
	}
}

func (s *Shell) finish() {
	p := s.pending
	s.pending = nil
	switch p.kind {
	case pendingChange:
		p.apply()
	case pendingPage:
		s.navigate(p.page, p.arrive)
	case pendingClose:
		s.window.ApproveClose()
	}
}

func (s *Shell) discard() {
	p := s.pending
	if p == nil {
		return
	}
	if _, busy := s.env.Activity.State(); busy {
		return
	}
	if p.kind == pendingChange {
		// The page stays: its sections revert their drafts in place.
		s.env.Activity.Discard()
	}
	s.env.Activity.Reset()
	s.finish()
}

func (s *Shell) cancel() {
	if s.pending != nil && s.pending.kind == pendingClose {
		s.window.CancelClose()
	}
	s.pending = nil
}

// View builds the window.
func (s *Shell) View(c *ui.Context) {
	dark := c.Theme().Dark
	switch s.snapshot.Settings.Theme {
	case "dark":
		dark = true
	case "light":
		dark = false
	}
	c.SetTheme(kitTheme(c, dark))
	ui.Column(c).Fill().Children(func() {
		s.titleBar(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).MinHeight(0).Children(func() {
			s.sidebar(c)
			s.content(c)
		})
	})
	s.settle()
	s.guard(c)
	for _, message := range s.toasts {
		c.Toast(message)
	}
	s.toasts = nil
	// Ctrl+S saves the page's current transaction, as its Save button.
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		if save := s.env.Rail.Save; s.env.Rail.Active() && !save.Disabled && !save.Busy && save.Run != nil {
			save.Run()
		}
	}
}

func (s *Shell) guard(c *ui.Context) {
	open := s.pending != nil
	if !open {
		return
	}
	_, busy := s.env.Activity.State()
	kind := s.pending.kind
	title, description := "navigation.dirtyTitle", "navigation.dirtyDescription"
	confirm := "navigation.discard"
	switch {
	case busy && kind == pendingClose:
		title, description = "navigation.waitTitle", "navigation.waitCloseDescription"
	case busy && kind == pendingChange:
		title, description = "navigation.waitTitle", "navigation.tabWaitDescription"
	case busy:
		title, description = "navigation.waitTitle", "navigation.waitDescription"
	case kind == pendingClose:
		description, confirm = "navigation.closeDescription", "navigation.discardClose"
	case kind == pendingChange:
		description = "navigation.tabDirtyDescription"
	}
	cancelLabel := i18n.T("navigation.keepEditing")
	confirmLabel := i18n.T(confirm)
	if busy {
		cancelLabel, confirmLabel = i18n.T("navigation.stay"), ""
	}
	if kitDialog(c, &open, i18n.T(title), i18n.T(description), s.pending.err, confirmLabel, cancelLabel) {
		s.discard()
		return
	}
	if !open {
		s.cancel()
	}
}

package settings

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/desktop"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/diagnostics"
)

// shellOpen opens the log folder or the release page with the system's
// handler; tests replace it.
var shellOpen = desktop.ShellOpen

func (p *page) about(c *ui.Context) {
	p.updates.view(c)
	connected := "shell.disconnected"
	if p.loaded && p.err == "" {
		connected = "shell.connected"
	}
	kit.Group(c, icons.Activity, i18n.T("shell.background"), "", func() {
		kit.Row(c, i18n.T("shell.connection"), i18n.T("shell.backgroundHint"), func() {
			kit.Status(c, i18n.T(connected)).SingleLine()
		})
	})
	p.logs.view(c)
}

// updateInfo is the build's version and whether it has an update source.
type updateInfo struct {
	Version    string `json:"version"`
	Configured bool   `json:"configured"`
}

// updateReply is the state of one update check (internal/updates.Reply).
type updateReply struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ErrorCode      string `json:"errorCode"`
}

// updater checks for a newer release through the Host, as the previous
// window did (component_bridge.rs CheckForUpdates): the Host runs one check
// at a time for this window's process, leaving the page cancels its check,
// and only a release that check confirmed can be opened.
type updater struct {
	env      *core.Env
	info     *updateInfo
	checking bool
	// id is the check running; last is the check whose release can be
	// opened, until it is cancelled.
	id, last string
	opening  bool
	result   *updateReply
	// err is an error code of a check, as "network".
	err string
}

var updateErrors = []string{"network", "timeout", "rate_limited", "not_found", "cancelled", "unavailable", "invalid_response", "invalid_version", "invalid_source", "busy", "open_failed"}

func (u *updater) load() {
	u.err = ""
	core.Go(u.env, func() (updateInfo, error) {
		var info updateInfo
		err := u.env.Host.CallInto("updates.info", nil, &info)
		return info, err
	}, func(info updateInfo, err error) {
		if err != nil {
			u.err = "unavailable"
			return
		}
		u.info = &info
	})
}

func (u *updater) check() {
	if u.checking || u.info == nil || !u.info.Configured {
		return
	}
	var random [16]byte
	_, _ = rand.Read(random[:])
	id := hex.EncodeToString(random[:])
	u.checking, u.id, u.last, u.err, u.result = true, id, id, "", nil
	host := u.env.Host
	core.Go(u.env, func() (updateReply, error) {
		var reply updateReply
		if err := host.CallInto("updates.check", map[string]string{"id": id}, &reply); err != nil {
			return reply, err
		}
		deadline := time.Now().Add(time.Minute)
		for reply.State == "checking" {
			if host.Cancelled() {
				return reply, core.Errorf("cancelled")
			}
			if !time.Now().Before(deadline) {
				_ = host.CallInto("updates.cancel", map[string]string{"id": id}, nil)
				return reply, core.Errorf("timeout")
			}
			time.Sleep(100 * time.Millisecond)
			if err := host.CallInto("updates.status", map[string]string{"id": id}, &reply); err != nil {
				return reply, err
			}
			if reply.ID != id {
				return reply, core.Errorf("protocol")
			}
		}
		return reply, nil
	}, func(reply updateReply, err error) {
		if u.id == id {
			u.checking, u.id = false, ""
		}
		switch {
		case err != nil:
			// The React page reported every failed request alike.
			u.err = "unavailable"
		case reply.State == "failed":
			u.err = reply.ErrorCode
			if u.err == "" {
				u.err = "unavailable"
			}
		case slices.Contains([]string{"available", "up_to_date", "not_configured"}, reply.State) && !(reply.State == "available" && reply.LatestVersion == ""):
			u.result = &reply
		default:
			u.err = "invalid_response"
		}
	})
}

// cancel stops the running check, whose result then reports "cancelled".
// It runs as the page closes too, so it does not wait for the Host.
func (u *updater) cancel() {
	id := u.id
	if id == "" {
		return
	}
	if u.last == id {
		u.last = ""
	}
	host := u.env.Host
	go func() { _ = host.CallInto("updates.cancel", map[string]string{"id": id}, nil) }()
}

func (u *updater) open() {
	if u.opening {
		return
	}
	u.opening, u.err = true, ""
	id, host := u.last, u.env.Host
	core.Do(u.env, func() error {
		if id == "" {
			return core.Errorf("no check")
		}
		var release struct {
			URL string `json:"url"`
		}
		if err := host.CallInto("updates.release", map[string]string{"id": id}, &release); err != nil {
			return err
		}
		if !releaseURL(release.URL) {
			return core.Errorf("invalid release")
		}
		return shellOpen(release.URL)
	}, func(err error) {
		u.opening = false
		if err != nil {
			u.err = "open_failed"
		}
	})
}

// releaseURL accepts only a GitHub release tag page, as the previous
// window's valid_release_url did.
func releaseURL(address string) bool {
	path, ok := strings.CutPrefix(address, "https://github.com/")
	if !ok {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[2] != "releases" || parts[3] != "tag" {
		return false
	}
	owner, repository, tag := parts[0], parts[1], parts[4]
	alnum := func(r byte) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' }
	only := func(s, extra string) bool {
		for i := 0; i < len(s); i++ {
			if !alnum(s[i]) && !strings.ContainsRune(extra, rune(s[i])) {
				return false
			}
		}
		return true
	}
	return owner != "" && len(owner) <= 39 && alnum(owner[0]) && alnum(owner[len(owner)-1]) && only(owner, "-") &&
		repository != "" && len(repository) <= 100 && repository != "." && repository != ".." && only(repository, "_.-") &&
		tag != "" && only(tag, ".-")
}

func (u *updater) view(c *ui.Context) {
	kit.Section(c, icons.RefreshCw, i18n.T("updates.title"), "", func() {
		kit.Panel(c, func() {
			label := i18n.T("updates.loading")
			if u.info != nil {
				label = i18n.T("updates.version", "version", u.info.Version)
			}
			help := ""
			if u.info != nil && !u.info.Configured || u.result != nil && u.result.State == "not_configured" {
				help = i18n.T("updates.notConfigured")
			}
			kit.Row(c, label, help, func() {
				if kit.Button(c, i18n.T("updates.check"), icons.RefreshCw, u.checking).Disabled(u.info == nil || !u.info.Configured).Clicked() {
					u.check()
				}
				if u.checking && kit.Button(c, i18n.T("updates.cancel"), icons.X, false).Clicked() {
					u.cancel()
				}
			})
			switch {
			case u.result != nil && u.result.State == "available":
				kit.Row(c, i18n.T("updates.available", "version", u.result.LatestVersion), "", func() {
					if kit.Button(c, i18n.T("updates.release"), icons.ExternalLink, u.opening).Clicked() {
						u.open()
					}
				}).Role(ui.RoleStatus)
			case u.result != nil && u.result.State == "up_to_date":
				kit.Row(c, i18n.T("updates.current"), "", nil).Role(ui.RoleStatus)
			}
		})
		switch {
		case u.err == "cancelled":
			kit.Status(c, i18n.T("updates.cancelled"))
		case u.err != "":
			key := u.err
			if !slices.Contains(updateErrors, key) {
				key = "unavailable"
			}
			// Without the version the check cannot start: offer to read it
			// again.
			var retry func()
			if u.info == nil {
				retry = func() {
					if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
						u.load()
					}
				}
			}
			kit.Notice(c, kit.Failure, "", i18n.T("updates."+key), retry)
		}
	})
}

// logs shows the end of WinOne's log from the profile, read by the window
// itself, so it works without the Host.
type logs struct {
	page       *page
	loading    bool
	opening    bool
	data       *diagnostics.ModuleLogs
	texts      map[string]*string
	err        string
	openErr    string
	generation int
}

func (l *logs) read() {
	l.generation++
	owner := l.generation
	l.loading, l.data, l.err = true, nil, ""
	env := l.page.env
	core.Go(env, func() (diagnostics.ModuleLogs, error) { return diagnostics.Read(env.Directory, "host") }, func(data diagnostics.ModuleLogs, err error) {
		if owner != l.generation {
			return
		}
		l.loading = false
		if err != nil {
			l.err = err.Error()
			return
		}
		l.data, l.texts = &data, map[string]*string{}
		for _, entry := range data.Entries {
			text := entry.Text
			l.texts[entry.Name] = &text
		}
	})
}

func (l *logs) open() {
	if l.opening {
		return
	}
	l.opening, l.openErr = true, ""
	env := l.page.env
	core.Do(env, func() error {
		folder, err := diagnostics.Folder(env.Directory, "host")
		if err != nil {
			return err
		}
		return shellOpen(folder)
	}, func(err error) {
		l.opening = false
		if err != nil {
			l.openErr = err.Error()
		}
	})
}

func (l *logs) view(c *ui.Context) {
	kit.Section(c, icons.FileText, i18n.T("diagnostics.title"), "", func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("diagnostics.refresh"), icons.RefreshCw, false).Disabled(l.loading).Clicked() {
				l.read()
			}
			available := l.data != nil && l.data.FolderAvailable
			if kit.Button(c, i18n.T("diagnostics.openFolder"), icons.FolderOpen, false).Disabled(l.opening || !available).Clicked() {
				l.open()
			}
		})
		if l.loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
				kit.Loader(c)
				kit.Help(c, i18n.T("diagnostics.loading"))
			})
		}
		if l.err != "" {
			kit.Notice(c, kit.Failure, "", l.err, nil)
		}
		if l.openErr != "" {
			kit.Notice(c, kit.Failure, "", l.openErr, nil)
		}
		if l.data == nil || l.loading {
			return
		}
		if len(l.data.Entries) == 0 {
			kit.Help(c, i18n.T("diagnostics.empty"))
		}
		t := c.Theme()
		for _, entry := range l.data.Entries {
			title := i18n.T("diagnostics." + entry.Name)
			ui.Column(c.Key(entry.Name)).FillWidth().Gap(6).Children(func() {
				kit.Strong(c, title).Role(ui.RoleHeading).Level(3)
				if entry.Truncated {
					kit.Help(c, i18n.T("diagnostics.truncated"))
				}
				if !entry.Available || entry.Text == "" {
					kit.Help(c, i18n.T("diagnostics.empty"))
					return
				}
				// A read-only text area lays out only the lines in view and
				// lets the log be selected and copied with the keyboard.
				ui.TextArea(c, l.texts[entry.Name]).ReadOnly(true).Label(title).Height(260).FillWidth().
					Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Background(t.Surface)
			})
		}
	})
}

package aiservices

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// signInPrompt is a question a plugin's sign-in asks: text, a password or
// a choice.
type signInPrompt struct {
	Type        string         `json:"type"`
	Key         string         `json:"key"`
	Message     string         `json:"message"`
	Placeholder string         `json:"placeholder,omitempty"`
	Options     []promptOption `json:"options,omitempty"`
}

type promptOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Hint  string `json:"hint,omitempty"`
}

// signIn is a sign-in the Host runs: the vendor's page, a device code,
// or something to paste back.
type signIn struct {
	ID     string `json:"id"`
	Status struct {
		Agent         string        `json:"agent"`
		URL           string        `json:"url"`
		Code          string        `json:"code,omitempty"`
		State         string        `json:"state"`
		User          string        `json:"user,omitempty"`
		Error         string        `json:"error,omitempty"`
		PasteCallback bool          `json:"pasteCallback,omitempty"`
		PasteCode     bool          `json:"pasteCode,omitempty"`
		PasteKey      bool          `json:"pasteKey,omitempty"`
		KeysURL       string        `json:"keysURL,omitempty"`
		Instructions  string        `json:"instructions,omitempty"`
		Opened        bool          `json:"opened,omitempty"`
		Prompt        *signInPrompt `json:"prompt,omitempty"`
	} `json:"status"`
	InputError   string `json:"inputError,omitempty"`
	InputPending bool   `json:"inputPending,omitempty"`
}

func (s *signIn) running() bool {
	return s != nil && s.Status.State != "done" && s.Status.State != "failed" && s.Status.State != "canceled"
}

// checkin is a daily check-in's result for one account.
type checkin struct {
	User    string   `json:"user,omitempty"`
	Outcome string   `json:"outcome"`
	Credit  *float64 `json:"credit,omitempty"`
	Streak  float64  `json:"streak,omitempty"`
	Asked   bool     `json:"asked,omitempty"`
}

// accountPanel is an account connection's sign-in. A sign-in belongs to
// the window: leaving the page or the connection cancels it. The vendor's
// page handles passwords; WinOne asks only for what the sign-in names.
type accountPanel struct {
	p   *page
	id  string
	req string // the request accounts were read for
	// lastRequest is the request a finished sign-in refreshes.
	lastRequest accountRequest

	accounts         []account
	methods          []signInMethod
	method           string
	checkinAvailable bool
	loading          bool
	err              string
	read             int

	job      *signIn
	starting bool
	action   string
	callback string
	choice   bool
	copied   string
	opened   string
	polling  bool
	// owner drops answers that a reset or close overtook.
	owner int

	checkins   []checkin
	checkedIn  bool
	checkingIn bool
	// signInNonce is the plugin request this panel last continued.
	signInNonce string
}

func newAccountPanel(p *page, pr *provider) *accountPanel {
	return &accountPanel{p: p, id: pr.ID}
}

// request is what the accounts are read for: a saved connection by ID,
// one not saved yet by its account type.
func (a *accountPanel) request(pr *provider) accountRequest {
	agentID := pr.Agent
	if agentID == "" {
		agentID = "codex"
	}
	r := accountRequest{Agent: agentID, Provider: pr.ID}
	if !a.p.connectionSaved(pr) {
		r.Draft = &draftAgent{Agent: agentID, AgentBackend: pr.AgentBackend}
	}
	return r
}

func requestKey(r accountRequest) string {
	key := r.Agent + "\x00" + r.Provider
	if r.Draft != nil {
		key += "\x00" + r.Draft.Agent + "\x00" + r.Draft.AgentBackend
	}
	return key
}

// reset starts over for another request, cancelling a sign-in.
func (a *accountPanel) reset() {
	a.cancelRunning()
	a.owner++
	a.read++
	a.accounts, a.methods, a.method, a.checkinAvailable = nil, nil, "", false
	a.checkins, a.checkedIn, a.checkingIn = nil, false, false
	a.starting, a.loading, a.action, a.copied, a.err = false, false, "", "", ""
	a.job = nil
	a.report()
}

func (a *accountPanel) cancelRunning() {
	if job := a.job; job.running() {
		id := job.ID
		spawn(a.p.env, func() { _ = a.p.env.Host.CallCompletingInto("capture.ai.accounts.signin.cancel", map[string]string{"id": id}, nil) })
	}
}

func (a *accountPanel) Close() {
	a.cancelRunning()
	a.owner++
	a.read++
	a.job = nil
	a.p.accountBusy, a.p.accountLoading = false, false
}

func (a *accountPanel) report() {
	a.p.accountBusy = a.job.running() || a.starting || a.checkingIn
	a.p.accountLoading = a.loading
}

// load reads the accounts; refresh reads them again from the Host.
func (a *accountPanel) load(r accountRequest, refresh bool) {
	a.read++
	owner := a.read
	a.loading, a.err = true, ""
	a.p.reader.Read(r, refresh, func(state accountState, err error) {
		if owner != a.read {
			return
		}
		a.loading = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.accounts, a.checkinAvailable, a.methods = state.Accounts, state.CheckinAvailable, state.Methods
		valid := false
		for i := range a.methods {
			valid = valid || kit.Itoa(i) == a.method
		}
		if !valid {
			a.method = "0"
			for i, m := range a.methods {
				if m.Type == "oauth" {
					a.method = kit.Itoa(i)
					break
				}
			}
		}
	})
}

// start begins a sign-in. A reply that arrives after the connection
// changed is cancelled at once.
func (a *accountPanel) start(r accountRequest) {
	p := a.p
	p.reader.Invalidate()
	a.err, a.starting, a.loading, a.callback, a.copied = "", true, true, "", ""
	owner := a.owner
	params := map[string]any{"agent": r.Agent, "provider": r.Provider}
	if r.Draft != nil {
		params["draft"] = r.Draft
	}
	if len(a.methods) > 0 {
		index := 0
		for i := range a.methods {
			if kit.Itoa(i) == a.method {
				index = i
			}
		}
		params["method"] = index
	}
	core.Go(p.env, func() (signIn, error) {
		var job signIn
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.signin.start", params, &job)
		return job, err
	}, func(job signIn, err error) {
		if owner != a.owner {
			if err == nil {
				id := job.ID
				spawn(p.env, func() { _ = p.env.Host.CallCompletingInto("capture.ai.accounts.signin.cancel", map[string]string{"id": id}, nil) })
			}
			return
		}
		a.loading, a.starting = false, false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.job = &job
		a.callback, a.choice = "", false
		a.poll(200 * time.Millisecond)
	})
}

// poll follows a running sign-in, opening its page once unless the
// plugin's own CLI opened it.
func (a *accountPanel) poll(after time.Duration) {
	p := a.p
	if a.polling || !a.job.running() {
		return
	}
	a.polling = true
	owner, id := a.owner, a.job.ID
	core.Go(p.env, func() (signIn, error) {
		time.Sleep(after)
		var next signIn
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.signin.get", map[string]string{"id": id}, &next)
		return next, err
	}, func(next signIn, err error) {
		a.polling = false
		if owner != a.owner || a.job == nil || a.job.ID != id {
			return
		}
		if err != nil {
			a.err = err.Error()
			a.job.Status.State = "failed"
			spawn(p.env, func() { _ = p.env.Host.CallCompletingInto("capture.ai.accounts.signin.cancel", map[string]string{"id": id}, nil) })
			return
		}
		previousPrompt := ""
		if a.job.Status.Prompt != nil {
			previousPrompt = a.job.Status.Prompt.Key
		}
		a.job = &next
		if next.Status.Prompt != nil && next.Status.Prompt.Key != previousPrompt {
			a.callback, a.choice = "", false
		}
		page := next.ID + ":" + next.Status.URL
		if next.running() && next.Status.URL != "" && a.opened != page {
			a.opened = page
			if !next.Status.Opened {
				spawn(p.env, func() {
					if p.env.Host.CallCompletingInto("capture.ai.accounts.signin.open", map[string]string{"id": id}, nil) != nil {
						p.env.Post(func() {
							if owner == a.owner {
								a.err = i18n.T("ai.openLoginFailed")
							}
						})
					}
				})
			}
		}
		if next.Status.State == "done" {
			a.load(a.lastRequest, true)
			return
		}
		a.poll(400 * time.Millisecond)
	})
}

func (a *accountPanel) cancel() {
	p := a.p
	if a.job == nil || a.action != "" {
		return
	}
	a.action = "cancel"
	owner, id := a.owner, a.job.ID
	core.Go(p.env, func() (signIn, error) {
		var next signIn
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.signin.cancel", map[string]string{"id": id}, &next)
		return next, err
	}, func(next signIn, err error) {
		if owner != a.owner {
			return
		}
		a.action = ""
		if err != nil {
			a.err = err.Error()
			return
		}
		a.job = &next
	})
}

// prompt helpers: what is asked, and whether the answer can be sent.
func (a *accountPanel) inputBusy() bool { return a.job != nil && (a.job.InputPending || a.action != "") }

func (a *accountPanel) canSubmit() bool {
	job := a.job
	if !job.running() {
		return false
	}
	if prompt := job.Status.Prompt; prompt != nil {
		if job.Status.State != "input" {
			return false
		}
		switch prompt.Type {
		case "text":
			return true
		case "select":
			return a.choice && slices.ContainsFunc(prompt.Options, func(o promptOption) bool { return o.Value == a.callback })
		}
		return strings.TrimSpace(a.callback) != ""
	}
	paste := job.Status.PasteCallback || job.Status.PasteCode || job.Status.PasteKey
	return job.Status.State == "waiting" && paste && strings.TrimSpace(a.callback) != ""
}

func (a *accountPanel) submit() {
	p := a.p
	if a.job == nil || !a.canSubmit() || a.inputBusy() {
		return
	}
	a.action, a.err = "submit", ""
	owner, id, callback := a.owner, a.job.ID, a.callback
	core.Go(p.env, func() (signIn, error) {
		var next signIn
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.signin.submit", map[string]string{"id": id, "callback": callback}, &next)
		return next, err
	}, func(next signIn, err error) {
		if owner != a.owner {
			return
		}
		a.action = ""
		if err != nil {
			a.err = err.Error()
			return
		}
		if next.ID == id && next.Status.State != "" {
			a.job = &next
			a.poll(400 * time.Millisecond)
		}
	})
}

func (a *accountPanel) checkIn(r accountRequest) {
	p := a.p
	a.checkingIn, a.err, a.checkins, a.checkedIn = true, "", nil, false
	owner := a.owner
	core.Go(p.env, func() ([]checkin, error) {
		var result struct {
			Checkins []checkin `json:"checkins"`
		}
		err := p.env.Host.CallCompletingInto("capture.ai.accounts.checkin", map[string]string{"agent": r.Agent, "provider": r.Provider}, &result)
		return result.Checkins, err
	}, func(list []checkin, err error) {
		if owner != a.owner {
			return
		}
		a.checkingIn = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.checkins, a.checkedIn = list, true
	})
}

// View is the account connection: its type, accounts, sign-in and
// check-in.
func (a *accountPanel) View(c *ui.Context, pr *provider, old *provider) {
	p := a.p
	r := a.request(pr)
	if key := requestKey(r); key != a.req {
		if a.req != "" {
			a.reset()
		}
		a.req = key
		a.lastRequest = r
		a.load(r, false)
	}
	a.lastRequest = r
	a.report()
	disabled := p.saving || p.loading || p.modelBusy
	active := a.job.running()
	if request := p.extensions.signIn; request != nil && request.provider == pr.ID && request.nonce != a.signInNonce &&
		a.method != "" && !a.loading && !disabled && !active && !a.starting {
		// The plugin manager's explicit sign-in continues here once the
		// methods are known, once: navigating never signs in.
		a.signInNonce = request.nonce
		p.extensions.signIn = nil
		a.start(r)
	}
	hidden := p.draft != nil && p.draft.Value.magpie().HideAccounts
	accountType := p.catalog.agent(r.Agent)
	name, icon := r.Agent, ""
	if accountType != nil {
		name, icon = accountType.Name, accountType.Icon
	}
	t := c.Theme()
	section := ui.Column(c).FillWidth().Gap(12).Role(ui.RoleGroup).Label(i18n.T("ai.accountConnection"))
	if p.focusAccount {
		p.focusAccount = false
		section.ScrollIntoView()
	}
	section.Children(func() {
		// The type was chosen when the connection was added: changing it
		// here could pair another agent with the connection's models.
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			widgets.ProviderIcon(c, p.env, icon, 20)
			kit.Heading(c, name)
		})
		if a.err != "" {
			kit.Notice(c, kit.Failure, "", a.err, nil)
		}
		kit.Panel(c, func() {
			if len(a.methods) > 1 {
				kit.Row(c, i18n.T("ai.signInMethod"), "", func() {
					var options []kit.SelectOption
					for i, m := range a.methods {
						options = append(options, kit.SelectOption{Value: kit.Itoa(i), Label: m.Label})
					}
					_, trigger := kit.Select(c, &a.method, options)
					trigger.Label(i18n.T("ai.signInMethod")).Disabled(disabled || active || a.loading || a.checkingIn)
				})
			}
			kit.Stack(c, "", "", func() {
				if len(a.accounts) > 0 {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						for i, acct := range a.accounts {
							ui.Column(c.Key(acct.Agent+acct.User)).Gap(2).MinWidth(0).Children(func() {
								user := acct.User
								if hidden {
									user = i18n.T("usage.maskedAccount", "number", i+1)
								}
								ui.Text(c, user).MinWidth(0)
								if acct.Plan != "" {
									kit.Help(c, acct.Plan)
								}
								if acct.Lapsed != "" {
									ui.Text(c, i18n.T("ai.signInAgain")).FontSize(kit.HelpSize(c)).TextColor(t.Warning)
								}
							})
						}
					})
				} else {
					status := "ai.noAccount"
					switch {
					case a.loading:
						status = "ai.loadingAccounts"
					case a.err != "":
						status = "ai.accountStatusUnknown"
					}
					kit.Status(c, i18n.T(status))
				}
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					lapsed := slices.ContainsFunc(a.accounts, func(x account) bool { return x.Lapsed != "" })
					label := "ai.signIn"
					if len(a.accounts) > 0 {
						label = "ai.reauthenticate"
					}
					var b ui.Element
					if len(a.accounts) == 0 || lapsed {
						b = kit.PrimaryButton(c, i18n.T(label), icons.LogIn, false)
					} else {
						b = kit.Button(c, i18n.T(label), icons.LogIn, false)
					}
					if b.Disabled(disabled || active || a.loading || a.checkingIn || r.Agent == "").Clicked() {
						a.start(r)
					}
					if kit.GhostButton(c, i18n.T("ai.refreshAccounts"), icons.RefreshCw, false).Disabled(disabled || active || a.loading || a.checkingIn).Clicked() {
						a.load(r, true)
					}
					if a.checkinAvailable && kit.GhostButton(c, i18n.T("ai.checkInNow"), icons.Check, a.checkingIn).
						Disabled(disabled || active || a.loading || a.checkingIn || r.Draft != nil).Clicked() {
						a.checkIn(r)
					}
				})
			})
		})
		if a.checkedIn {
			a.checkinView(c, hidden)
		}
		if a.job != nil {
			a.jobView(c)
		}
		kit.Help(c, i18n.T("ai.accountScope"))
	})
}

var checkinOutcomes = map[string]string{"done": "ai.checkinDone", "inactive": "ai.checkinInactive", "ineligible": "ai.checkinIneligible",
	"own-app": "ai.checkinOwnApp", "captcha": "ai.checkinCaptcha", "failed": "ai.checkinFailed"}

func (a *accountPanel) checkinView(c *ui.Context, hidden bool) {
	ui.Column(c).FillWidth().Gap(4).Role(ui.RoleStatus).Children(func() {
		if len(a.checkins) == 0 {
			kit.Help(c, i18n.T("ai.checkinEmpty"))
			return
		}
		for i, item := range a.checkins {
			user := item.User
			if hidden {
				user = i18n.T("usage.maskedAccount", "number", i+1)
			}
			key := checkinOutcomes[item.Outcome]
			switch {
			case item.Outcome == "claimed" && item.Asked:
				key = "ai.checkinClaimed"
			case item.Outcome == "claimed":
				key = "ai.checkinEarlier"
			case key == "":
				key = "ai.checkinUnknown"
			}
			line := user + " · " + i18n.T(key)
			if item.Credit != nil {
				line += " · " + i18n.T("ai.checkinCredits", "count", formatNumber(*item.Credit))
			}
			if item.Streak != 0 {
				line += " · " + i18n.T("ai.checkinStreak", "count", formatNumber(item.Streak))
			}
			ui.Text(c.Key(i), line).FontSize(kit.HelpSize(c))
		}
	})
}

// jobView is a sign-in in progress or ended: what to do next, the
// device code, the login link, what to paste back.
func (a *accountPanel) jobView(c *ui.Context) {
	job := a.job
	s := &job.Status
	active := job.running()
	prompt := s.Prompt
	busy := a.inputBusy()
	services := c.Services()
	copy := func(text, kind string) {
		services.WriteClipboard(text)
		a.copied = kind
	}
	kit.Panel(c, func() {
		kit.Stack(c, "", "", func() {
			if active && prompt == nil {
				hint := "ai.signInWaiting"
				switch {
				case s.State == "starting":
					hint = "ai.signInStarting"
				case s.State == "finishing":
					hint = "ai.signInFinishing"
				case s.Code != "":
					hint = "ai.deviceCodeHint"
				case s.PasteKey:
					hint = "ai.authorizationKeyHint"
				case s.PasteCode:
					hint = "ai.authorizationCodeHint"
				case s.PasteCallback:
					hint = "ai.callbackHint"
				case s.URL != "":
					hint = "ai.signInBrowser"
				}
				kit.Status(c, i18n.T(hint))
			}
			switch s.State {
			case "done":
				kit.Status(c, i18n.T("ai.signedIn"))
			case "canceled":
				kit.Status(c, i18n.T("ai.signInCanceled"))
			case "failed":
				if s.Error == "" {
					kit.Notice(c, kit.Failure, "", i18n.T("ai.signInFailed"), nil)
				}
			}
			if s.Error != "" {
				kit.Notice(c, kit.Failure, "", i18n.T("ai.signInFailureDetails", "detail", s.Error), nil)
			}
			if active && s.Instructions != "" {
				kit.Help(c, s.Instructions)
			}
			if job.InputError != "" {
				kit.Notice(c, kit.Failure, "", job.InputError, nil)
			}
			if active && prompt != nil {
				a.promptView(c, prompt, busy)
			}
			if active && s.Code != "" {
				readonlyCopy(c, i18n.T("ai.deviceCode"), s.Code, i18n.T("ai.copyDeviceCode"), a.copied == "device", true, func() { copy(s.Code, "device") })
			}
			if active && s.URL != "" {
				readonlyCopy(c, i18n.T("ai.loginLink"), s.URL, i18n.T("ai.copyLoginLink"), a.copied == "link", false, func() { copy(s.URL, "link") })
			}
			if active && prompt == nil && (s.PasteCallback || s.PasteCode || s.PasteKey) {
				label := "ai.callback"
				switch {
				case s.PasteKey:
					label = "ai.authorizationKey"
				case s.PasteCode:
					label = "ai.authorizationCode"
				}
				ui.Field(c, i18n.T(label), func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						input := ui.TextInput(c, &a.callback).Grow(1).MinWidth(0).Label(i18n.T(label)).Disabled(busy)
						if s.PasteKey {
							input.Password()
						}
						if input.Submitted() {
							a.submit()
						}
						if kit.Button(c, i18n.T("ai.submitCode"), icons.Check, busy).Disabled(!a.canSubmit()).Clicked() {
							a.submit()
						}
					})
				})
			}
			if active {
				id := job.ID
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					if s.KeysURL != "" && kit.Button(c, i18n.T("ai.openKeys"), icons.ExternalLink, false).Clicked() {
						a.open(id, true)
					}
					if s.URL != "" && kit.Button(c, i18n.T("ai.openLogin"), icons.ExternalLink, false).Clicked() {
						a.open(id, false)
					}
					if kit.Button(c, i18n.T("ai.cancelLogin"), icons.X, a.action == "cancel").Clicked() {
						a.cancel()
					}
				})
			}
		})
	})
}

// open opens the sign-in's page, or its keys page, again.
func (a *accountPanel) open(id string, keys bool) {
	p := a.p
	owner := a.owner
	params := map[string]any{"id": id}
	if keys {
		params["keys"] = true
	}
	core.Do(p.env, func() error {
		return p.env.Host.CallCompletingInto("capture.ai.accounts.signin.open", params, nil)
	}, func(err error) {
		if err == nil || owner != a.owner {
			return
		}
		if keys {
			a.err = err.Error()
		} else {
			a.err = i18n.T("ai.openLoginFailed")
		}
	})
}

// promptView answers a plugin's question: a choice, or text.
func (a *accountPanel) promptView(c *ui.Context, prompt *signInPrompt, busy bool) {
	ui.Field(c, prompt.Message, func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			if prompt.Type == "select" {
				var options []kit.SelectOption
				hint := ""
				for _, o := range prompt.Options {
					options = append(options, kit.SelectOption{Value: o.Value, Label: o.Label})
					if o.Value == a.callback {
						hint = o.Hint
					}
				}
				changed, trigger := kit.Select(c, &a.callback, options)
				trigger.Label(prompt.Message).Disabled(busy)
				if changed {
					a.choice = true
				}
				if hint != "" {
					kit.Help(c, hint)
				}
			} else {
				input := ui.TextInput(c, &a.callback).FillWidth().Label(prompt.Message).Placeholder(prompt.Placeholder).Disabled(busy)
				if prompt.Type == "password" {
					input.Password()
				}
				if input.Submitted() {
					a.submit()
				}
			}
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("ai.continueSignIn"), icons.Check, busy).Disabled(!a.canSubmit()).Clicked() {
					a.submit()
				}
			})
		})
	})
}

// readonlyCopy shows a value to copy: selectable, with a copy button that
// shows a check once used.
func readonlyCopy(c *ui.Context, label, text, copyLabel string, copied, mono bool, copy func()) {
	ui.Field(c, label, func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			value := text
			input := ui.TextInput(c, &value).Grow(1).MinWidth(0).ReadOnly(true).Label(label)
			if mono {
				input.Font(kit.MonoFont)
			}
			icon := icons.Copy
			if copied {
				icon = icons.Check
			}
			if kit.IconButton(c, icon, copyLabel).Clicked() {
				copy()
			}
		})
	})
}

// formatNumber writes a number as JavaScript did: no trailing zeros.
func formatNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

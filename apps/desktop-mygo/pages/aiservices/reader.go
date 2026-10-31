package aiservices

import (
	"encoding/json"
	"sync"
	"time"

	"winone/desktopmygo/core"
)

// agent is an account type the Host can sign in to: a built-in agent or a
// plugin's.
type agent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Icon    string `json:"icon,omitempty"`
	Backend string `json:"backend,omitempty"`
	Plugin  string `json:"plugin,omitempty"`
}

// endpoints are a preset's addresses by API.
type endpoints struct {
	Chat      string `json:"chat,omitempty"`
	Responses string `json:"responses,omitempty"`
	Anthropic string `json:"anthropic,omitempty"`
}

func (e endpoints) of(protocol string) string {
	switch protocol {
	case "chat":
		return e.Chat
	case "responses":
		return e.Responses
	case "anthropic":
		return e.Anthropic
	}
	return ""
}

type presetRegion struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	endpoints
}

// servicePreset is a known service: its endpoints, models and notes.
type servicePreset struct {
	endpoints
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Icon      string            `json:"icon,omitempty"`
	Models    []string          `json:"models,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Note      string            `json:"note,omitempty"`
	Short     string            `json:"short,omitempty"`
	NoKey     bool              `json:"noKey,omitempty"`
	Sponsored bool              `json:"sponsored,omitempty"`
	Website   string            `json:"website,omitempty"`
	SiteHost  string            `json:"siteHost,omitempty"`
	KeysURL   string            `json:"keysUrl,omitempty"`
	Langs     []string          `json:"langs,omitempty"`
	Notes     map[string]string `json:"notes,omitempty"`
	Regions   []presetRegion    `json:"regions,omitempty"`
}

// presetConnection is the preset's endpoint for the preferred API, or for
// the first API the region offers. The user can still edit the address.
func presetConnection(p servicePreset, region, preferred string) (protocol, baseURL string) {
	e := p.endpoints
	for _, r := range p.Regions {
		if r.ID == region {
			e = r.endpoints
		}
	}
	protocol = ""
	for _, choice := range []string{"chat", "responses", "anthropic"} {
		if choice == preferred && e.of(choice) != "" {
			protocol = choice
		}
	}
	if protocol == "" {
		for _, choice := range []string{"chat", "responses", "anthropic"} {
			if e.of(choice) != "" {
				protocol = choice
				break
			}
		}
	}
	if protocol == "" {
		protocol = "chat"
	}
	baseURL = e.of(protocol)
	if baseURL == "" {
		baseURL = p.Endpoint
	}
	return protocol, baseURL
}

// accountCatalog is what accounts the Host can add, and the presets.
type accountCatalog struct {
	Agents   []agent         `json:"agents"`
	Presets  []servicePreset `json:"presets,omitempty"`
	Partners []servicePreset `json:"partners,omitempty"`
}

// account is a signed-in account's public summary.
type account struct {
	Agent  string `json:"agent"`
	User   string `json:"user"`
	Plan   string `json:"plan,omitempty"`
	Lapsed string `json:"lapsed,omitempty"`
	Icon   string `json:"icon,omitempty"`
}

// signInMethod is a way to sign in: the vendor's page, or an API key.
type signInMethod struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
}

type accountState struct {
	Accounts         []account      `json:"accounts"`
	Methods          []signInMethod `json:"methods,omitempty"`
	CheckinAvailable bool           `json:"checkinAvailable,omitempty"`
}

// accountRequest names the accounts to list: those of a saved provider,
// or of an account type a draft connection has not saved yet.
type accountRequest struct {
	Agent    string       `json:"agent"`
	Provider string       `json:"provider,omitempty"`
	Draft    *draftAgent  `json:"draft,omitempty"`
}

type draftAgent struct {
	Agent        string `json:"agent"`
	AgentBackend string `json:"agentBackend,omitempty"`
}

// accountReader shares the Host's account reads across tabs and visits of
// the page, as React's createAccountReader did for the window: a read in
// flight is joined, a finished one is reused for 30 seconds, the catalog
// for an hour. Refreshing and signing in invalidate them.
type accountReader struct {
	env     *core.Env
	mu      sync.Mutex
	catalog *catalogRead
	entries map[string]*accountRead
}

type catalogRead struct {
	done  chan struct{}
	value accountCatalog
	err   error
	at    time.Time
}

type accountRead struct {
	done    chan struct{}
	value   accountState
	err     error
	expires time.Time
}

var readers sync.Map // *core.Env → *accountReader

// readerFor returns the window's reader, one per environment.
func readerFor(env *core.Env) *accountReader {
	r, _ := readers.LoadOrStore(env, &accountReader{env: env, entries: map[string]*accountRead{}})
	return r.(*accountReader)
}

// Catalog reads the account types and presets, then calls done on the UI
// thread. An install that invalidates the catalog while it is read gives
// its callers the newer list.
func (r *accountReader) Catalog(done func(accountCatalog, error)) {
	r.mu.Lock()
	if r.catalog != nil && !r.catalog.at.IsZero() && time.Since(r.catalog.at) >= time.Hour {
		r.catalog = nil
	}
	read := r.catalog
	if read == nil {
		read = &catalogRead{done: make(chan struct{})}
		r.catalog = read
		go func() {
			var value accountCatalog
			err := r.env.Host.CallCompletingInto("capture.ai.accounts.catalog", map[string]any{}, &value)
			if err == nil && len(value.Partners) > 0 {
				value.Presets = append(append([]servicePreset{}, value.Partners...), value.Presets...)
			}
			r.mu.Lock()
			read.value, read.err = value, err
			if err == nil {
				read.at = time.Now()
			} else if r.catalog == read {
				r.catalog = nil
			}
			r.mu.Unlock()
			close(read.done)
		}()
	}
	r.mu.Unlock()
	spawn(r.env, func() {
		<-read.done
		r.mu.Lock()
		stale := r.catalog != read && r.catalog != nil
		r.mu.Unlock()
		if stale {
			r.Catalog(done)
			return
		}
		r.env.Post(func() { done(read.value, read.err) })
	})
}

func (r *accountReader) InvalidateCatalog() {
	r.mu.Lock()
	r.catalog = nil
	r.mu.Unlock()
}

// Read lists accounts. refresh reads again unless a read is in flight.
func (r *accountReader) Read(request accountRequest, refresh bool, done func(accountState, error)) {
	key, _ := json.Marshal([]any{request.Provider, request.Agent, draftField(request.Draft, true), draftField(request.Draft, false)})
	r.mu.Lock()
	read := r.entries[string(key)]
	pending := read != nil && read.expires.IsZero()
	if read == nil || !(pending || !refresh && time.Now().Before(read.expires)) {
		read = &accountRead{done: make(chan struct{})}
		r.entries[string(key)] = read
		go func() {
			var value accountState
			err := r.env.Host.CallCompletingInto("capture.ai.accounts.list", request, &value)
			r.mu.Lock()
			read.value, read.err = value, err
			if err == nil {
				read.expires = time.Now().Add(30 * time.Second)
			} else if r.entries[string(key)] == read {
				delete(r.entries, string(key))
			}
			r.mu.Unlock()
			close(read.done)
		}()
	}
	r.mu.Unlock()
	spawn(r.env, func() {
		<-read.done
		r.env.Post(func() { done(read.value, read.err) })
	})
}

func draftField(d *draftAgent, agent bool) string {
	if d == nil {
		return ""
	}
	if agent {
		return d.Agent
	}
	return d.AgentBackend
}

func (r *accountReader) Invalidate() {
	r.mu.Lock()
	r.entries = map[string]*accountRead{}
	r.mu.Unlock()
}

// spawn starts background work the way core.Go does, so tests know when
// it finished.
func spawn(env *core.Env, fn func()) {
	if env.Spawn != nil {
		env.Spawn(fn)
		return
	}
	go fn()
}

// catalogState is the page's copy of the catalog.
type catalogState struct {
	agents  []agent
	presets []servicePreset
	loaded  bool
	busy    bool
	err     string
}

func (c *catalogState) preset(id string) *servicePreset {
	if id == "" {
		return nil
	}
	for i := range c.presets {
		if c.presets[i].ID == id {
			return &c.presets[i]
		}
	}
	return nil
}

func (c *catalogState) agent(id string) *agent {
	for i := range c.agents {
		if c.agents[i].ID == id {
			return &c.agents[i]
		}
	}
	return nil
}

func (p *page) loadCatalog() {
	if p.catalog.busy {
		return
	}
	p.catalog.busy, p.catalog.err = true, ""
	p.reader.Catalog(func(value accountCatalog, err error) {
		p.catalog.busy = false
		if err != nil {
			p.catalog.err = err.Error()
			return
		}
		p.catalog.agents, p.catalog.presets, p.catalog.loaded = value.Agents, value.Presets, true
	})
}

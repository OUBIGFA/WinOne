package aiservices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The external pages never share the ordinary AI worker's private client home.
// A lease is requested only after an explicit Read click, and every later API
// request is one user action (there are no scan, update or install effects in View).
type externalPanel struct {
	p                         *page
	url, viewer, err, message string
	working                   bool
	generation                uint64
	cancel                    context.CancelFunc
	library                   map[string]any
	tab                       int
	query                     string
	selected                  map[string]bool
	edit                      *libraryEditor
	confirm                   *libraryAction
	result                    any
	sessions                  externalSessions
	market                    []map[string]any
	marketKind, marketQuery   string
	rtk                       map[string]any
	skill                     *librarySkillFile
	service                   struct {
		Enabled, Browser, Listening, Exporting bool
		URL                                    string
	}
}
type libraryAction struct {
	title, path string
	message     string
	body        any
	done        func(json.RawMessage)
}
type libraryEditor struct {
	kind, old               string
	values                  map[string]string
	original                map[string]any
	agents                  map[string]bool
	chosen                  map[string][]string
	serverView              int
	serverJSON, serverError string
	serverFields            map[string]any
	serverText              map[string]string
}
type librarySkillFile struct {
	Name, File, Text, Was, Note string
	Files                       []string
}

func newExternalPanel(p *page) *externalPanel {
	return &externalPanel{p: p, selected: map[string]bool{}, marketKind: "servers", sessions: externalSessions{limit: 50}}
}
func (x *externalPanel) busy() bool { return x.working }
func (x *externalPanel) dirty() bool {
	return x.edit != nil || x.skill != nil && x.skill.Text != x.skill.Was || x.sessions.relocation != nil
}
func (x *externalPanel) discard() {
	x.edit = nil
	x.skill = nil
	x.confirm = nil
	x.sessions.relocation = nil
}
func (x *externalPanel) close() {
	x.p.env.Activity.Report("ai-external", false, false, nil)
	x.generation++
	if x.cancel != nil {
		x.cancel()
		x.cancel = nil
	}
	x.working = false
	if x.viewer != "" {
		viewer := x.viewer
		x.viewer = ""
		core.Go(x.p.env, func() (bool, error) {
			return true, x.p.env.Host.CallCompletingInto("capture.ai.external.close", map[string]string{"viewer": viewer}, nil)
		}, func(bool, error) {})
	}
	x.url = ""
	x.sessions.exporting = false
}
func (x *externalPanel) open(kind string) {
	x.connect(func() { x.load(kind) })
}

// Every caller is an explicit user action. Sharing the viewer lease keeps
// plugin tools and Library operations on the same private external service.
func (x *externalPanel) connect(done func()) {
	if x.working {
		return
	}
	if x.url != "" {
		done()
		return
	}
	x.working = true
	x.err = ""
	x.viewer = nonce()
	viewer := x.viewer
	ticket := x.generation
	core.Go(x.p.env, func() (struct {
		URL string `json:"url"`
	}, error) {
		var state struct {
			URL string `json:"url"`
		}
		err := x.p.env.Host.CallCompletingInto("capture.ai.external.open", map[string]string{"viewer": viewer}, &state)
		return state, err
	}, func(state struct {
		URL string `json:"url"`
	}, err error) {
		if ticket != x.generation {
			return
		}
		x.working = false
		if err != nil {
			x.err = err.Error()
			return
		}
		base, e := url.Parse(state.URL)
		if e != nil || base.Scheme != "http" || base.Hostname() != "127.0.0.1" || base.User != nil {
			x.err = nativeT("invalidRuntime")
			return
		}
		x.url = state.URL
		done()
	})
}
func (x *externalPanel) request(method, path string, body any, done func(json.RawMessage)) {
	if x.working || x.url == "" {
		return
	}
	base, err := url.Parse(x.url)
	if err != nil || base.Scheme != "http" || base.Hostname() != "127.0.0.1" || !strings.HasPrefix(path, "/api/") {
		x.err = nativeT("invalidRuntime")
		return
	}
	address := base.ResolveReference(&url.URL{Path: "./"}).String() + strings.TrimPrefix(path, "/")
	payload, err := json.Marshal(body)
	if err != nil {
		x.err = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	x.cancel = cancel
	x.working = true
	x.err = ""
	ticket := x.generation
	core.Go(x.p.env, func() (json.RawMessage, error) {
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New(nativeT("redirectRefused")) }}
		res, err := client.Do(req)
		if err != nil {
			var e *url.Error
			if errors.As(err, &e) {
				err = e.Err
			}
			return nil, err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 32<<20 {
			return nil, errors.New(nativeT("responseTooLarge"))
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			var fail struct{ Error string }
			_ = json.Unmarshal(data, &fail)
			if fail.Error == "" {
				fail.Error = strings.TrimSpace(string(data))
			}
			return nil, fmt.Errorf("%s (HTTP %d)", fail.Error, res.StatusCode)
		}
		if res.StatusCode == 204 || len(data) == 0 {
			return json.RawMessage(`{}`), nil
		}
		if strings.Contains(res.Header.Get("Content-Type"), "text/markdown") {
			b, _ := json.Marshal(string(data))
			return b, nil
		}
		if !json.Valid(data) {
			return nil, errors.New(nativeT("invalidResponse"))
		}
		return data, nil
	}, func(data json.RawMessage, err error) {
		if ticket != x.generation {
			return
		}
		x.working = false
		x.cancel = nil
		if err != nil {
			x.err = err.Error()
			return
		}
		if done != nil {
			done(data)
		}
	})
}
func (x *externalPanel) load(kind string) {
	x.request("GET", "/api/winone/external/status", nil, func(raw json.RawMessage) {
		if err := json.Unmarshal(raw, &x.service); err != nil {
			x.err = err.Error()
			return
		}
		x.sessions.exporting = x.service.Exporting
		x.loadContent(kind)
	})
}

func (x *externalPanel) loadContent(kind string) {
	if kind == "sessions" {
		x.loadSessions()
		return
	}
	x.request("GET", "/api/library", nil, x.acceptLibrary)
}
func (x *externalPanel) acceptLibrary(raw json.RawMessage) {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		x.err = nativeT("invalidResponse")
		return
	}
	if textOf(value, "part") == "instructions" && value["instructions"] != nil && x.library != nil {
		x.library["instructions"] = value["instructions"]
		x.library["problems"] = value["problems"]
	} else {
		if value["agents"] == nil || value["servers"] == nil || value["skills"] == nil {
			x.err = nativeT("invalidResponse")
			return
		}
		x.library = value
	}
	x.edit = nil
	x.message = externalT("saved")
	x.result = value["result"]
}
func (x *externalPanel) act(path string, body any) {
	x.request("POST", "/api/library/"+path, body, x.acceptLibrary)
}
func (x *externalPanel) ask(title, path string, body any, done func(json.RawMessage)) {
	x.confirm = &libraryAction{title: title, path: path, body: body, done: done}
}
func (x *externalPanel) view(c *ui.Context, kind string) {
	x.p.env.Activity.Report("ai-external", x.dirty(), x.busy(), x.discard)
	ui.Column(c).FillWidth().Gap(18).Children(func() { x.viewContent(c, kind) })
}

func (x *externalPanel) viewContent(c *ui.Context, kind string) {
	kit.Help(c, externalT("boundary"))
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		label := externalT("read")
		if x.url != "" {
			label = upstreamT("Refresh")
		}
		if kit.PrimaryButton(c, label, icons.RefreshCw, x.working).Disabled(x.working || x.dirty()).Clicked() {
			x.open(kind)
		}
		if x.url != "" && kit.Button(c, externalT("disconnect"), nil, false).Disabled(x.working || x.dirty()).Clicked() {
			x.close()
		}
	})
	if x.err != "" {
		kit.Notice(c, kit.Failure, "", x.err, func() {
			if kit.Button(c, i18n.T("common.close"), nil, false).Clicked() {
				x.err = ""
			}
		})
	}
	if x.message != "" {
		kit.Help(c, x.message)
		if kit.Button(c, i18n.T("common.close"), nil, false).Clicked() {
			x.message = ""
		}
	}
	if x.url == "" {
		return
	}
	if x.service.Enabled {
		ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			kit.Help(c, externalT("clientServiceRunning"))
			if x.service.Browser && kit.Button(c, externalT("stopBrowser"), nil, false).Disabled(x.working).Clicked() {
				x.request("POST", "/api/winone/external/browser/stop", nil, func(raw json.RawMessage) {
					if err := json.Unmarshal(raw, &x.service); err != nil {
						x.err = err.Error()
					}
				})
			}
		})
	}
	if kind == "sessions" {
		x.sessionsView(c)
	} else {
		x.libraryView(c)
	}
	if x.confirm != nil {
		a := x.confirm
		open := true
		if kit.Dialog(c, &open, kit.DialogOptions{Title: a.title, Confirm: upstreamT("Confirm"), Cancel: i18n.T("common.cancel"), Busy: x.working, Error: x.err, Width: 600, Body: func() {
			message := a.message
			if message == "" {
				message = externalT("writeBoundary")
			}
			kit.Help(c, message)
		}}) {
			x.request("POST", a.path, a.body, func(raw json.RawMessage) {
				if a.done != nil {
					a.done(raw)
				}
				if x.err == "" {
					x.confirm = nil
				}
			})
		}
		if !open && !x.working {
			x.confirm = nil
		}
	}
}

func textOf(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}
func boolOf(m map[string]any, key string) bool             { v, _ := m[key].(bool); return v }
func objectOf(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
func rowsOf(m map[string]any, key string) []map[string]any {
	items, _ := m[key].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if v, ok := item.(map[string]any); ok {
			out = append(out, v)
		}
	}
	return out
}
func stringsOf(value any) []string {
	items, _ := value.([]any)
	out := []string{}
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if v, ok := value.([]string); ok {
		return v
	}
	return out
}
func picked(values map[string]bool) []string {
	out := []string{}
	for key, on := range values {
		if on {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}
func selectedStrings(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		out[v] = true
	}
	return out
}
func listLines(value string) []string {
	out := []string{}
	for _, v := range strings.Split(strings.ReplaceAll(value, "\r", ""), "\n") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
func mapLines(value string) map[string]string {
	out := map[string]string{}
	for _, line := range listLines(value) {
		key, v, ok := strings.Cut(line, "=")
		if ok {
			out[strings.TrimSpace(key)] = v
		}
	}
	return out
}
func mapText(value any) string {
	m, _ := value.(map[string]any)
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []string{}
	for _, key := range keys {
		out = append(out, key+"="+fmt.Sprint(m[key]))
	}
	return strings.Join(out, "\n")
}

// Readable native key/value groups are also used for variable upstream health
// receipts; editing always uses typed fields below, never a JSON textarea.
func externalDetails(c *ui.Context, value any) {
	switch v := value.(type) {
	case nil:
		return
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			switch v[key].(type) {
			case map[string]any, []any:
				kit.Strong(c, upstreamT(key))
				externalDetails(c, v[key])
			default:
				kit.Help(c, upstreamT(key)+": "+fmt.Sprint(v[key]))
			}
		}
	case []any:
		for _, item := range v {
			externalDetails(c, item)
		}
	default:
		kit.Help(c, fmt.Sprint(v))
	}
}

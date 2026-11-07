package aiservices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// nativeAPI talks to the same Host-owned, capability-authenticated Magpie
// process used by plugin pages. The native window never chooses another host,
// forwards its token across redirects, or invents a second routing registry.
// Long plugin installs keep their upstream five-minute budget, not IPC's 15s.
type nativeAPI struct {
	busy       bool
	err        string
	generation uint64
	cancel     context.CancelFunc
}

func (a *nativeAPI) close() {
	a.generation++
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.busy = false
}

func (a *nativeAPI) request(p *page, method, endpoint string, body any, done func(json.RawMessage)) {
	if a.busy || p.extensions.state == nil {
		return
	}
	base, err := url.Parse(p.extensions.state.URL)
	if err != nil || base.Hostname() != "127.0.0.1" || base.Scheme != "http" || base.User != nil {
		a.err = i18n.T("aiNative.invalidRuntime")
		return
	}
	if !strings.HasPrefix(endpoint, "/api/") {
		a.err = i18n.T("aiNative.invalidRuntime")
		return
	}
	// Preserve /session/<capability>/ when resolving an API, not the public root.
	address := base.ResolveReference(&url.URL{Path: "./"}).String() + strings.TrimPrefix(endpoint, "/")
	payload, err := json.Marshal(body)
	if err != nil {
		a.err = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.cancel = cancel
	a.generation++
	ticket := a.generation
	a.busy, a.err = true, ""
	core.Go(p.env, func() (json.RawMessage, error) {
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New(nativeT("redirectRefused")) }}
		response, err := client.Do(request)
		if err != nil {
			// net/http includes the capability URL in url.Error.Error(). Keep
			// the real network cause but never expose a live session capability.
			var urlError *url.Error
			if errors.As(err, &urlError) {
				err = urlError.Err
			}
			return nil, fmt.Errorf("%s", i18n.T("aiNative.requestFailed", "error", err.Error()))
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 8<<20 {
			return nil, errors.New(i18n.T("aiNative.responseTooLarge"))
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			var problem struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(data, &problem)
			message := problem.Error
			if message == "" {
				message = strings.TrimSpace(string(data))
			}
			return nil, fmt.Errorf("%s (HTTP %d)", message, response.StatusCode)
		}
		if !json.Valid(data) {
			return nil, errors.New(i18n.T("aiNative.invalidResponse"))
		}
		return data, nil
	}, func(data json.RawMessage, err error) {
		if ticket != a.generation {
			return
		}
		a.busy, a.cancel = false, nil
		if err != nil {
			a.err = err.Error()
			return
		}
		done(data)
	})
}

func (a *nativeAPI) notice(c *ui.Context, retry func()) {
	if a.err != "" {
		kit.Notice(c, kit.Failure, "", a.err, func() {
			if retry != nil && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, a.busy).Disabled(a.busy).Clicked() {
				retry()
			}
		})
	}
}

// Native diagnostics keep complete upstream data, selectable and copyable,
// without loading HTML. Structured controls remain the primary editing surface.
func nativeJSON(c *ui.Context, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		kit.Mono(c, string(data)).FillWidth().MinWidth(0).Selectable()
	}
}

func nativeDecode[T any](a *nativeAPI, raw json.RawMessage, value *T) bool {
	if err := json.Unmarshal(raw, value); err != nil {
		a.err = err.Error()
		return false
	}
	return true
}

// Empty arrays are meaningful; a missing/null required collection is not a
// successful mutation receipt. Keep drafts when an upstream contract changes.
func decodeRouteGroups(a *nativeAPI, raw json.RawMessage, value *routeGroups) bool {
	if !nativeDecode(a, raw, value) {
		return false
	}
	if value.Groups == nil {
		a.err = nativeT("invalidResponse")
		return false
	}
	return true
}
func decodeManagedPlugins(a *nativeAPI, raw json.RawMessage, value *managedPlugins) bool {
	if !nativeDecode(a, raw, value) {
		return false
	}
	if value.Plugins == nil {
		a.err = nativeT("invalidResponse")
		return false
	}
	return true
}

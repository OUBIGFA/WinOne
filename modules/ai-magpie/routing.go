package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/gui"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// The shared router keeps Magpie's live trace, affinity and cooldown state
// across calls. The Host owns the authenticated loopback listener and process.
func serveRouting(ctx context.Context, in request, input io.Reader, output io.Writer) error {
	if len(in.Key) < 32 {
		return errors.New("missing routing key")
	}
	// Only routing's own saved name choice overlays the fresh Host snapshot.
	// Never restore old credentials, provider endpoints or unrelated settings.
	var savedNames *settings.Settings
	if _, err := os.Stat(settings.Path()); err == nil {
		s, readErr := settings.LoadChecked()
		if readErr != nil {
			return readErr
		}
		savedNames = &s
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := applyRuntimeSettings(in); err != nil {
		return err
	}
	if savedNames != nil {
		s := settings.Load()
		s.PlainNames = savedNames.PlainNames
		s.PlainOwnNames = savedNames.PlainOwnNames
		settings.WinOneUse(s)
	}
	if err := configureUsage(in); err != nil {
		return err
	}
	provider.WinOneProviders = []provider.Provider{}
	if err := installAuxiliaryProviders(in); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	gw := gateway.New()
	stopPlugins := serveBuiltinPlugins()
	defer stopPlugins()
	bundled := make([]string, 0, len(accountPlugins()))
	for _, p := range accountPlugins() {
		bundled = append(bundled, p.Package)
	}
	page := gui.WinOneRoutingHandler(gw, bundled)
	protocol := gw.Handler()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		// A per-process capability path also authenticates iframe assets when
		// WebView blocks third-party cookies. It never authorizes other hosts.
		if strings.HasPrefix(r.URL.Path, "/session/") {
			capability, rest, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/session/"), "/")
			if ok && subtle.ConstantTimeCompare([]byte(capability), []byte(in.Key)) == 1 {
				token = capability
				r = r.Clone(r.Context())
				target := *r.URL
				target.Path = "/" + rest
				target.RawPath = ""
				r.URL = &target
				r = gui.WinOneWithPagePrefix(r, "/session/"+capability)
			}
		}
		if token == "" {
			cookie, _ := r.Cookie("winone-routing")
			if cookie != nil {
				token = cookie.Value
			}
		}
		if token == "" && r.URL.Path == "/" {
			token = r.URL.Query().Get("key")
			if subtle.ConstantTimeCompare([]byte(token), []byte(in.Key)) == 1 {
				q := r.URL.Query()
				q.Del("key")
				http.Redirect(w, r, "/session/"+token+"/?"+q.Encode(), 303)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(in.Key)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/mcp/magpie/web-search" || r.URL.Path == "/mcp/magpie/tools" {
			r.Header.Set("Authorization", "Bearer "+gateway.Token)
			protocol.ServeHTTP(w, winOneCallerRequest(r))
			return
		}
		// Browser writes must come from this embedded page; external origins do
		// not gain authorization from the loopback cookie.
		if r.Method == http.MethodPost && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "Invalid origin", 403)
			return
		}
		page.ServeHTTP(w, r)
	})
	go func() { <-ctx.Done(); server.Close() }()
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, input); server.Close(); close(done) }()
	if err = json.NewEncoder(output).Encode(map[string]string{"url": "http://" + listener.Addr().String()}); err != nil {
		listener.Close()
		return err
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

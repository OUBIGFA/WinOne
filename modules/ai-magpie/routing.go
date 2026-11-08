package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/gui"
	"github.com/yetone/magpie/internal/pii"
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
	if in.Operation == "privacy.serve" {
		// Model management must not enumerate accounts, execute installed
		// extensions or require Bun just because its status page was opened.
		settings.WinOneUse(settings.Settings{NoStats: true, NoUsageStats: true, NoWSLAgents: true, RedactModelOff: true})
	} else if err := applyRuntimeSettings(in); err != nil {
		return err
	}
	if savedNames != nil {
		s := settings.Load()
		s.PlainNames = savedNames.PlainNames
		s.PlainOwnNames = savedNames.PlainOwnNames
		if in.Operation == "privacy.serve" {
			s.RedactModel = savedNames.RedactModel
			s.RedactModelOff = !s.RedactModel
			s.RedactModelName, s.RedactModelScore = savedNames.RedactModelName, savedNames.RedactModelScore
			// Existing opt-ins selected Privacy Filter before models were named.
			if s.RedactModel && s.RedactModelName == "" {
				s.RedactModelName = pii.PrivacyFilter
			}
		}
		settings.WinOneUse(s)
	}
	defer func() {
		for _, m := range pii.Models() {
			m.Cancel()
			m.Stop()
		}
	}()
	provider.WinOneProviders = []provider.Provider{}
	var page, protocol http.Handler
	if in.Operation == "privacy.serve" {
		page = gui.WinOnePrivacyHandler()
	} else {
		if err := configureUsage(in); err != nil {
			return err
		}
		if err := installAuxiliaryProviders(in); err != nil {
			return err
		}
		gw := gateway.New()
		stopPlugins := serveBuiltinPlugins()
		defer stopPlugins()
		bundled := make([]string, 0, len(accountPlugins()))
		for _, p := range accountPlugins() {
			bundled = append(bundled, p.Package)
		}
		page, protocol = gui.WinOneRoutingHandler(gw, bundled), gw.Handler()
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
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
		if in.Operation == "routing.serve" && r.URL.Path == "/api/winone/catalog" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+in.Key || r.Header.Get("Origin") != "" {
				http.Error(w, "Unauthorized", 401)
				return
			}
			var update struct {
				ID         string                   `json:"id"`
				Models     []string                 `json:"models"`
				Metadata   map[string]modelMetadata `json:"metadata"`
				Contexts   map[string]int           `json:"contexts"`
				ModelNames map[string]string        `json:"modelNames"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&update) != nil || decoder.Decode(new(any)) != io.EOF || update.ID == "" {
				http.Error(w, "Invalid catalog", 400)
				return
			}
			contexts := update.Contexts
			if contexts == nil {
				contexts = map[string]int{}
			}
			for id, metadata := range update.Metadata {
				if err := installModelPreferences(update.ID, id, metadata); err != nil {
					http.Error(w, "Invalid model settings", 400)
					return
				}
				if metadata.ContextWindow > 0 {
					contexts[id] = metadata.ContextWindow
				}
				if metadata.Alias != "" && contexts[id] > 0 {
					contexts[metadata.Alias] = contexts[id]
				}
			}
			if err := installModelNames(update.ID, update.ModelNames, update.Metadata); err != nil {
				http.Error(w, "Invalid model names", 400)
				return
			}
			provider.WinOneUpdateCatalog(update.ID, update.Models, contexts)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if in.Operation == "routing.serve" && r.URL.Path == "/api/winone/privacy" {
			// Hot binding is Host-only: page cookies/capability paths and browser
			// origins cannot select an inference endpoint or change its opt-in.
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+in.Key || r.Header.Get("Origin") != "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			var binding struct{ URL, Key string }
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&binding) != nil || decoder.Decode(new(any)) != io.EOF {
				http.Error(w, "Invalid privacy binding", http.StatusBadRequest)
				return
			}
			if err := pii.WinOneUseRemote(binding.URL, binding.Key); err != nil {
				http.Error(w, "Invalid privacy binding", http.StatusBadRequest)
				return
			}
			settings.WinOneUseRedactModel(binding.URL != "")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if in.Operation != "privacy.serve" && (r.URL.Path == "/me" || r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/v1beta/models") || strings.HasPrefix(r.URL.Path, "/backend-api/codex/") || r.URL.Path == "/api/hello" || r.URL.Path == "/muse-code/models" || r.URL.Path == "/api/version" || r.URL.Path == "/api/tags" || r.URL.Path == "/api/show" || r.URL.Path == "/api/chat" || r.URL.Path == "/api/generate" || strings.HasPrefix(r.URL.Path, "/mcp/")) {
			r.Header.Set("Authorization", "Bearer "+gateway.Token)
			r = winOneCallerRequest(r)
			var authenticated bool
			r, authenticated = gateway.WinOneNamedRequest(w, r)
			if !authenticated {
				return
			}
			if r.Header.Get("X-WinOne-Product") == "1" {
				r = gateway.WinOneProductRequest(r, r.Header.Get("X-WinOne-Session"))
			}
			r.Header.Del("X-WinOne-Product")
			r.Header.Del("X-WinOne-Session")
			if raw := r.Header.Get("X-WinOne-Media-Models"); raw != "" {
				r.Header.Del("X-WinOne-Media-Models")
				var models []string
				if json.Unmarshal([]byte(raw), &models) != nil || len(models) == 0 {
					http.Error(w, "Invalid media model policy", 403)
					return
				}
				r = gateway.WinOneMediaRequest(r, models)
			}
			protocol.ServeHTTP(w, r)
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

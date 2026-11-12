package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// One Host-owned worker serves gateway protocols, keys and its private native
// administration. No plugin pages, route groups or external-client service.
func serveGatewayBackend(ctx context.Context, in request, input io.Reader, output io.Writer) error {
	if len(in.Key) < 32 {
		return errors.New("missing gateway key")
	}
	if err := applyRuntimeSettings(in); err != nil {
		return err
	}
	if err := configureAccounting(in); err != nil {
		return err
	}
	provider.WinOneProviders = []provider.Provider{}
	if err := installAuxiliaryProviders(in); err != nil {
		return err
	}
	stop := serveGatewayLimits()
	defer stop()
	gw := gateway.New()
	page, protocol := gatewayAdminHandler(gw), gw.Handler()
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
			}
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(in.Key)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/api/winone/catalog" {
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
		if IsGatewayServicePath(r.URL.Path) {
			r.Header.Set("Authorization", "Bearer "+gateway.Token)
			r = winOneCallerRequest(r)
			var authenticated bool
			r, authenticated = gateway.WinOneNamedRequest(w, r)
			if !authenticated {
				return
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

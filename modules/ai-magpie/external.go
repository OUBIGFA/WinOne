package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/browserext"
	"github.com/yetone/magpie/internal/gui"
	"github.com/yetone/magpie/internal/library"
	"github.com/yetone/magpie/internal/otlp"
	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"
)

type externalGateway struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// External management owns no model/account runtime and performs no warm-up.
// Client files are touched only by the explicitly requested upstream handler.
func serveExternal(ctx context.Context, in request, input io.Reader, output io.Writer) error {
	if len(in.Key) < 32 {
		return errors.New("missing external service key")
	}
	if err := appdir.WinOneExternalRoot(os.Getenv("WINONE_AI_EXTERNAL_DATA")); err != nil {
		return err
	}
	browserext.WinOneExternal = true
	// Listing the library uses cached names. Its MCP listener validates and
	// starts the shared plugin runtime only for an actual tools request.
	defer plugin.Settle()
	if in.ExternalGateway != nil {
		library.WinOneExternalUseGateway(in.ExternalGateway.URL, in.ExternalGateway.Key)
	}
	prefs, err := settings.LoadChecked()
	if err != nil {
		return err
	}
	prefs.NoStats = true
	prefs.NoUsageStats = true
	prefs.RedactModelOff = true
	prefs.OTel = settings.OTel{}
	if err := settings.Save(prefs); err != nil {
		return err
	}
	settings.WinOneExternalOTel(&settings.OTel{})
	defer settings.WinOneExternalOTel(nil)
	provider.WinOneProviders = []provider.Provider{}
	service, err := loadExternalService()
	if err != nil {
		return err
	}
	defer service.close()
	if service.Enabled {
		if err := service.start(); err != nil {
			return err
		}
	}
	handler := gui.WinOneExternalHandler()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	var exportMu sync.Mutex
	var stopExport func()
	defer func() {
		exportMu.Lock()
		defer exportMu.Unlock()
		if stopExport != nil {
			stopExport()
		}
	}()
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/session/" + in.Key + "/"
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if strings.HasPrefix(r.URL.Path, prefix) {
			token = in.Key
			r = r.Clone(r.Context())
			u := *r.URL
			u.Path = "/" + strings.TrimPrefix(u.Path, prefix)
			u.RawPath = ""
			r.URL = &u
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(in.Key)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Forbidden", 403)
			return
		}
		if r.URL.Path == "/api/winone/external/status" && r.Method == http.MethodGet {
			// OAuth may finish on its callback between page polls. Reconcile
			// private Library/sign-in state before the Host decides residency.
			if err := service.refresh(false); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			state := service.status()
			exportMu.Lock()
			state["exporting"] = stopExport != nil
			exportMu.Unlock()
			json.NewEncoder(w).Encode(state)
			return
		}
		if r.URL.Path == "/api/winone/external/browser/stop" && r.Method == http.MethodPost {
			if err := service.stopBrowser(); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			json.NewEncoder(w).Encode(service.status())
			return
		}
		if r.URL.Path == "/api/winone/external/settings" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+in.Key || r.Header.Get("Origin") != "" {
				http.Error(w, "Unauthorized", 401)
				return
			}
			var update struct {
				On             bool `json:"on"`
				MagpieSettings struct {
					OTel settings.OTel `json:"otel"`
				} `json:"magpieSettings"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&update) != nil {
				http.Error(w, "Invalid exporter settings", 400)
				return
			}
			exportMu.Lock()
			defer exportMu.Unlock()
			if update.On {
				if !update.MagpieSettings.OTel.Enabled || update.MagpieSettings.OTel.Endpoint == "" {
					http.Error(w, "Save an enabled OpenTelemetry destination first", 400)
					return
				}
				if err := update.MagpieSettings.OTel.Check(); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
			}
			if stopExport != nil {
				stopExport()
				stopExport = nil
			}
			config := update.MagpieSettings.OTel
			config.Enabled = update.On
			config.Sessions = update.On
			settings.WinOneExternalOTel(&config)
			if update.On {
				stopExport = otlp.New().Serve()
			}
			json.NewEncoder(w).Encode(map[string]bool{"on": update.On})
			return
		}
		if r.URL.Path == "/api/winone/external/gateway" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+in.Key || r.Header.Get("Origin") != "" {
				http.Error(w, "Unauthorized", 401)
				return
			}
			var endpoint externalGateway
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&endpoint) != nil {
				http.Error(w, "Invalid gateway endpoint", 400)
				return
			}
			library.WinOneExternalUseGateway(endpoint.URL, endpoint.Key)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		if r.URL.Path == "/api/library" || strings.HasPrefix(r.URL.Path, "/api/library/") {
			if r.Method == http.MethodPost && needsExternalListener(r.URL.Path) || r.Method == http.MethodGet && r.URL.Path == "/api/library" {
				if err := service.start(); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
			}
			if r.Method == http.MethodPost && r.URL.Path == "/api/library/servers/plugin-on" {
				// A plugin can be offered from Installed before Library was opened.
				// Adopt its cached tool names without scanning client directories.
				if err := library.WinOneExternalAdoptTools(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			receipt := httptest.NewRecorder()
			handler.ServeHTTP(receipt, r)
			if err := service.refresh(receipt.Code >= 200 && receipt.Code < 300 && r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/library/browser/")); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			for key, values := range receipt.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(receipt.Code)
			w.Write(receipt.Body.Bytes())
			return
		}
		handler.ServeHTTP(w, r)
	})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if err := json.NewEncoder(output).Encode(map[string]string{"url": "http://" + listener.Addr().String()}); err != nil {
		server.Close()
		return err
	}
	go func() { io.Copy(io.Discard, input); server.Close() }()
	select {
	case <-ctx.Done():
		server.Close()
		return ctx.Err()
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

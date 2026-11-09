package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/browserext"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/library"
	"github.com/yetone/magpie/internal/mcpauth"
	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/steady"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// An explicitly configured client service survives closing its settings page.
// Its stable port/key are private WinOne state; restoring the listener reads no
// client folders. Library handlers and MCP calls remain the only readers.
type externalService struct {
	mu          sync.Mutex
	Port        int    `json:"port"`
	Key         string `json:"key"`
	Enabled     bool   `json:"enabled"`
	Browser     bool   `json:"browser"`
	BrowserOff  bool   `json:"browserOff,omitempty"`
	server      *http.Server
	protocol    *gateway.Server
	connections sync.Map // Includes browser WebSockets after HTTP hijacking.
	pluginOnce  sync.Once
	pluginErr   error
}

func externalServiceFile() string { return filepath.Join(appdir.Config(), "external-service.json") }
func loadExternalService() (*externalService, error) {
	s := &externalService{}
	data, err := os.ReadFile(externalServiceFile())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Port < 0 || s.Port > 65535 || s.Key != "" && len(s.Key) < 32 {
		return nil, errors.New("invalid external service state")
	}
	return s, nil
}
func (s *externalService) save() error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(externalServiceFile()), 0700); err != nil {
		return err
	}
	return steady.WriteFile(externalServiceFile(), data, 0600)
}
func (s *externalService) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked()
}
func (s *externalService) startLocked() error {
	if s.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)))
	if err != nil {
		return err
	}
	if s.Key == "" {
		s.Key = rand.Text() + rand.Text()
	}
	s.Port = listener.Addr().(*net.TCPAddr).Port
	// All mutations and calls share this one permanent address. Environment is
	// changed once in this dedicated process before its first library mutation.
	if err := os.Setenv("MAGPIE_ADDR", listener.Addr().String()); err != nil {
		listener.Close()
		return err
	}
	agent.WinOneExternalKey = s.Key
	browserext.WinOneExternalKey = s.Key
	if err := s.save(); err != nil {
		listener.Close()
		return err
	}
	s.protocol = gateway.New()
	protocol := s.protocol.Handler()
	s.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ConnState: func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			s.connections.Store(conn, true)
		}
		if state == http.StateClosed {
			s.connections.Delete(conn)
		}
	}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		proxyName := strings.TrimPrefix(path, "/mcp/")
		allowed := strings.HasPrefix(path, gateway.HubMCPPath) || path == gateway.HubMCPAll || strings.HasPrefix(path, "/mcp/run/") || path == gateway.BrowserMCPPath || path == gateway.ToolsMCPPath || strings.HasPrefix(path, gateway.ToolsMCPOne) || strings.HasPrefix(path, "/mcp/") && !strings.Contains(proxyName, "/") && proxyName != "" || path == gateway.BrowserExtensionPath || path == gateway.BrowserStatusPath
		if !allowed {
			http.NotFound(w, r)
			return
		}
		if path == gateway.BrowserMCPPath || path == gateway.BrowserExtensionPath || path == gateway.BrowserStatusPath {
			s.mu.Lock()
			off := s.BrowserOff
			s.mu.Unlock()
			if off {
				http.Error(w, "Browser service is disabled", http.StatusServiceUnavailable)
				return
			}
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if path == gateway.BrowserExtensionPath {
			token = r.URL.Query().Get("key")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Key)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if path != gateway.BrowserExtensionPath && r.Header.Get("Origin") != "" {
			http.Error(w, "Forbidden", 403)
			return
		}
		r = r.Clone(r.Context())
		r.Header = r.Header.Clone()
		r.Header.Set("Authorization", "Bearer "+gateway.Token)
		// Upstream also reads the extension's query key. Its own guard should see
		// only the authenticated local identity, never try our private capability.
		if path == gateway.BrowserExtensionPath {
			u := *r.URL
			q := u.Query()
			q.Set("key", gateway.Token)
			u.RawQuery = q.Encode()
			r.URL = &u
		}
		// A plugin can be installed after this long-lived service opened.
		// Validate the bundled runtime before its first actual tools request.
		pluginRequest := path == gateway.ToolsMCPPath || strings.HasPrefix(path, gateway.ToolsMCPOne) || strings.HasPrefix(path, gateway.HubMCPPath) || path == gateway.HubMCPAll
		if pluginRequest && len(plugin.Load().Plugins) > 0 {
			s.pluginOnce.Do(func() { s.pluginErr = prepareThirdPartyRuntime("external.serve") })
			if s.pluginErr != nil {
				http.Error(w, s.pluginErr.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		protocol.ServeHTTP(w, r)
	})}
	go s.server.Serve(listener)
	return nil
}
func (s *externalService) stopLocked() {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
	s.connections.Range(func(key, value any) bool { key.(net.Conn).Close(); s.connections.Delete(key); return true })
	if gateway.MCPHub != nil {
		gateway.MCPHub.Stop()
	}
}
func (s *externalService) close() { s.mu.Lock(); defer s.mu.Unlock(); s.stopLocked() }
func (s *externalService) stopBrowser() error {
	s.mu.Lock()
	s.Browser = false
	s.BrowserOff = true
	protocol := s.protocol
	s.mu.Unlock()
	if protocol != nil {
		protocol.WinOneExternalStopBrowser()
	}
	// A browser WebSocket can be connected before its hello frame registers it
	// with the upstream relay. Close the owned loopback connections as well.
	s.connections.Range(func(key, value any) bool { key.(net.Conn).Close(); return true })
	return s.refresh(false)
}
func (s *externalService) refresh(browser bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lib library.Library
	data, err := os.ReadFile(filepath.Join(appdir.Config(), "library.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &lib); err != nil {
			return err
		}
	}
	if browser {
		s.BrowserOff = false
	}
	s.Browser = (s.Browser || browser) && !s.BrowserOff
	s.Enabled = lib.MCPHub || lib.SkillRouter || s.Browser
	for _, server := range lib.MCP {
		if server == nil || server.Hidden {
			continue
		}
		if !s.BrowserOff && len(server.Args) == 2 && server.Args[0] == "mcp" && server.Args[1] == "browser" {
			s.Enabled = true
		}
		if server.Plugin != "" && len(server.Agents) > 0 || server.Transport == "http" && mcpauth.SignedIn(server.Name, server.URL) {
			s.Enabled = true
		}
	}
	if err := s.save(); err != nil {
		return err
	}
	if !s.Enabled {
		s.stopLocked()
	} else if s.server == nil {
		return s.startLocked()
	}
	return nil
}
func (s *externalService) status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"enabled": s.Enabled, "browser": s.Browser, "url": gateway.URL(), "listening": s.server != nil}
}
func needsExternalListener(path string) bool {
	return strings.HasPrefix(path, "/api/library/servers/") || path == "/api/library/market/server" || path == "/api/library/mcp-signin" || path == "/api/library/skills/router" || strings.HasPrefix(path, "/api/library/browser/")
}

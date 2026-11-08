//go:build windows

package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"winone/internal/settings"
)

// Gateway settings belong to this machine, not portable AI provider preferences.
type GatewaySettings struct {
	GatewayNetwork
	Revision uint64 `json:"revision"`
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Native   bool   `json:"native"` // Explicit full provider API access; model filters cannot constrain unknown vendor endpoints.
	// nil preserves the legacy set; an explicit empty slice exposes no models.
	Models []Selection `json:"models"`
}
type GatewayStatus struct {
	GatewaySettings
	Running bool     `json:"running"`
	HasKey  bool     `json:"hasKey"`
	BaseURL string   `json:"baseUrl"`
	LANURLs []string `json:"lanUrls,omitempty"`
	Error   string   `json:"error,omitempty"`
}
type gatewayDocument struct {
	GatewaySettings
	Key string `json:"protectedKey"`
}
type Gateway struct {
	mu       sync.Mutex
	path     string
	ctx      context.Context
	handler  http.Handler
	external http.Handler // Named-key authentication remains in the shared AI runtime.
	document gatewayDocument
	server   *http.Server
	cancel   context.CancelFunc
	failure  string
	requests sync.WaitGroup
	paused   bool
	models   map[Selection]struct{}
}

type gatewayCallerKey struct{}

// Only successful local authentication creates this identity. Never use a
// caller-supplied header or persist the secret itself in usage records.
func GatewayCallerKeyID(ctx context.Context) string {
	id, _ := ctx.Value(gatewayCallerKey{}).(string)
	return id
}

func NewGateway(ctx context.Context, path string, handler http.Handler, external ...http.Handler) *Gateway {
	g := &Gateway{ctx: ctx, path: path, handler: handler, document: gatewayDocument{GatewaySettings: GatewaySettings{Port: 3426}}}
	if len(external) > 0 {
		g.external = external[0]
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) > maximumSettingsBytes || json.Unmarshal(data, &g.document) != nil || g.document.Port < 1024 || g.document.Port > 65535 {
			g.failure = "本地网关配置无效，请恢复配置文件"
			return g
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		g.failure = "无法读取本地网关配置"
		return g
	}
	g.models, err = gatewayModelSet(g.document.Models)
	if err == nil {
		err = g.document.GatewayNetwork.validate()
	}
	if err != nil {
		g.failure = err.Error()
		return g
	}
	if g.document.Enabled {
		if err := g.listenLocked(g.document); err != nil {
			g.failure = err.Error()
		}
	}
	go func() { <-ctx.Done(); g.Close() }()
	return g
}

func (g *Gateway) Status() GatewayStatus { g.mu.Lock(); defer g.mu.Unlock(); return g.statusLocked() }
func (g *Gateway) statusLocked() GatewayStatus {
	settings := g.document.GatewaySettings
	settings.Models = slices.Clone(settings.Models)
	settings.AllowIPs = slices.Clone(settings.AllowIPs)
	settings.CORSOrigins = slices.Clone(settings.CORSOrigins)
	out := GatewayStatus{GatewaySettings: settings, Running: g.server != nil, HasKey: g.document.Key != "", BaseURL: "http://127.0.0.1:" + strconv.Itoa(g.document.Port) + "/v1", Error: g.failure}
	if settings.LAN {
		if addresses, err := net.InterfaceAddrs(); err == nil {
			for _, address := range addresses {
				if ip, ok := address.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
					out.LANURLs = append(out.LANURLs, "http://"+net.JoinHostPort(ip.IP.String(), strconv.Itoa(settings.Port))+"/v1")
				}
			}
		}
	}
	return out
}
func (g *Gateway) Key() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return unsealKey(g.document.Key)
}

// Bind before committing configuration; a busy port cannot silently replace a
// running endpoint. A failed disk write closes the candidate and retains the old one.
func (g *Gateway) Save(value GatewaySettings, reset bool) (GatewayStatus, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fail := func(err error) (GatewayStatus, error) { return g.statusLocked(), err }
	if g.ctx.Err() != nil {
		return fail(errors.New("WinOne 正在退出"))
	}
	if g.paused {
		return fail(errors.New("WinOne 正在维护，请稍后重试"))
	}
	if value.Revision != g.document.Revision {
		return fail(settings.ErrConflict)
	}
	if value.Port < 1024 || value.Port > 65535 {
		return fail(errors.New("端口必须为 1024 到 65535 之间的整数"))
	}
	if err := value.GatewayNetwork.validate(); err != nil {
		return fail(err)
	}
	value.AllowIPs = slices.Clone(value.AllowIPs)
	value.CORSOrigins = slices.Clone(value.CORSOrigins)
	// Older forms do not know the model field. They cannot reopen routes that
	// the user disabled with a newer form; the UI sends [] to disable all.
	if value.Models == nil {
		value.Models = g.document.Models
	}
	value.Models = slices.Clone(value.Models)
	models, err := gatewayModelSet(value.Models)
	if err != nil {
		return fail(err)
	}
	next := gatewayDocument{GatewaySettings: value, Key: g.document.Key}
	next.Revision++
	if reset || next.Key == "" {
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return fail(err)
		}
		encrypted, err := protect([]byte("winone-"+hex.EncodeToString(token[:])), false)
		if err != nil {
			return fail(err)
		}
		next.Key = base64.StdEncoding.EncodeToString(encrypted)
	}
	oldServer, oldCancel := g.server, g.cancel
	changed := value.Enabled && (g.server == nil || value.Port != g.document.Port || value.LAN != g.document.LAN)
	// A bind-address change on the same port cannot be prepared beside the old
	// listener on Windows. Close it, and restore it if the candidate fails.
	rebind := changed && oldServer != nil && value.Port == g.document.Port
	if rebind {
		oldCancel()
		oldServer.Close()
		g.server, g.cancel = nil, nil
	}
	if changed {
		if err := g.listenLocked(next); err != nil {
			if rebind {
				if restore := g.listenLocked(g.document); restore != nil {
					g.failure = restore.Error()
				}
			}
			return fail(err)
		}
	}
	data, err := json.Marshal(next)
	if err == nil && len(data) > maximumSettingsBytes {
		err = errors.New("网关模型配置过大，请减少选择的模型")
	}
	if err == nil {
		err = settings.WriteAtomic(g.path, data)
	}
	if err != nil {
		if changed {
			g.cancel()
			g.server.Close()
			g.server, g.cancel = oldServer, oldCancel
			if rebind {
				g.server, g.cancel = nil, nil
				if restore := g.listenLocked(g.document); restore != nil {
					g.failure = restore.Error()
				}
			}
		}
		return fail(err)
	}
	g.document = next
	g.models = models
	g.failure = ""
	if !value.Enabled {
		g.server, g.cancel = nil, nil
	}
	if oldServer != nil && (changed || !value.Enabled) {
		oldCancel()
		oldServer.Close()
	}
	return g.statusLocked(), nil
}

func (g *Gateway) listenLocked(document gatewayDocument) error {
	if _, err := unsealKey(document.Key); err != nil || document.Key == "" {
		return errors.New("本地网关访问密钥不可用，请重置密钥")
	}
	address := "127.0.0.1"
	if document.LAN {
		address = "0.0.0.0"
	}
	listener, err := net.Listen("tcp4", address+":"+strconv.Itoa(document.Port))
	if err != nil {
		return errors.New("本地网关端口不可用，请更换端口后保存")
	}
	ctx, cancel := context.WithCancel(g.ctx)
	server := &http.Server{Handler: http.HandlerFunc(g.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	g.server, g.cancel = server, cancel
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.mu.Lock()
			if g.server == server {
				g.failure = "本地网关已停止，请重新保存设置"
				g.server = nil
			}
			g.mu.Unlock()
		}
	}()
	return nil
}

func GatewayError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message, "type": "winone_gateway_error"}})
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	network := g.document.GatewayNetwork
	enabled := g.document.Enabled && g.server != nil
	secret, err := unsealKey(g.document.Key)
	if enabled && err == nil {
		g.requests.Add(1)
	}
	g.mu.Unlock()
	if !enabled || err != nil {
		GatewayError(w, 503, "本地网关未启用")
		return
	}
	defer g.requests.Done()
	if !network.allowsPeer(r.RemoteAddr, r.Host) || !network.allowsOrigin(r.Header.Get("Origin")) {
		GatewayError(w, 403, "客户端地址或浏览器来源未获允许")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Goog-API-Key, Anthropic-Version, Anthropic-Beta, OpenAI-Beta, MCP-Protocol-Version, MCP-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "MCP-Session-Id, Retry-After, X-Request-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if supplied == "" {
		supplied = r.Header.Get("x-api-key")
	}
	if supplied == "" {
		supplied = r.Header.Get("x-goog-api-key")
	}
	if supplied == "" {
		supplied = r.URL.Query().Get("key")
	}
	// Only trusted context carries public credentials beyond this listener.
	// Neither the vendor transport nor a protocol adapter should receive them.
	r = r.Clone(r.Context())
	r.URL.Path = gatewayCanonicalPath(r.URL.Path)
	r.Header.Del("Origin")
	r.Header.Del("Authorization")
	r.Header.Del("X-API-Key")
	r.Header.Del("X-Goog-API-Key")
	query := r.URL.Query()
	query.Del("key")
	r.URL.RawQuery = query.Encode()
	// The upstream key page is a static login document, with no account or
	// usage data. Its separate data endpoint still requires a valid key.
	if r.URL.Path == "/me" && r.URL.RawPath == "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) && g.external != nil {
		g.external.ServeHTTP(w, r)
		return
	}
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(supplied)) != 1 {
		if supplied != "" && g.external != nil {
			// This handler must authenticate before exposing any service data.
			r = r.Clone(context.WithValue(r.Context(), gatewayCredentialKey{}, supplied))
			g.external.ServeHTTP(w, r)
			return
		}
		GatewayError(w, 401, "本地网关访问密钥无效")
		return
	}
	id := sha256.Sum256([]byte(secret))
	g.handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gatewayCallerKey{}, hex.EncodeToString(id[:]))))
}

type gatewayCredentialKey struct{}

// GatewayCredential exists only after the public listener's network checks;
// the shared runtime still must prove the named key and enforce its policy.
func GatewayCredential(ctx context.Context) string {
	key, _ := ctx.Value(gatewayCredentialKey{}).(string)
	return key
}

func (g *Gateway) Close() {
	g.mu.Lock()
	g.paused = true
	if g.server != nil {
		g.cancel()
		g.server.Close()
		g.server = nil
	}
	g.mu.Unlock()
	g.requests.Wait()
}

// A failed maintenance operation restores the user's enabled endpoint.
func (g *Gateway) Resume() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paused = false
	if g.document.Enabled && g.server == nil && g.ctx.Err() == nil {
		err := g.listenLocked(g.document)
		if err != nil {
			g.failure = err.Error()
		}
		return err
	}
	return nil
}

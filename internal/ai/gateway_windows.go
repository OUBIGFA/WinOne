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
	Revision uint64 `json:"revision"`
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	// nil preserves the legacy set; an explicit empty slice exposes no models.
	Models []Selection `json:"models"`
}
type GatewayStatus struct {
	GatewaySettings
	Running bool   `json:"running"`
	HasKey  bool   `json:"hasKey"`
	BaseURL string `json:"baseUrl"`
	Error   string `json:"error,omitempty"`
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

func NewGateway(ctx context.Context, path string, handler http.Handler) *Gateway {
	g := &Gateway{ctx: ctx, path: path, handler: handler, document: gatewayDocument{GatewaySettings: GatewaySettings{Port: 3426}}}
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
	return GatewayStatus{settings, g.server != nil, g.document.Key != "", "http://127.0.0.1:" + strconv.Itoa(g.document.Port) + "/v1", g.failure}
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
	changed := value.Enabled && (g.server == nil || value.Port != g.document.Port)
	if changed {
		if err := g.listenLocked(next); err != nil {
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
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(document.Port))
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
	// Browser origins require an explicit product policy; local native clients
	// need no CORS. Never expose this loopback service through DNS rebinding.
	name, _, err := net.SplitHostPort(r.Host)
	if err != nil || name != "127.0.0.1" && name != "localhost" || r.Header.Get("Origin") != "" {
		GatewayError(w, 403, "仅支持本机客户端调用")
		return
	}
	g.mu.Lock()
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
	supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if supplied == "" {
		supplied = r.Header.Get("x-api-key")
	}
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(supplied)) != 1 {
		GatewayError(w, 401, "本地网关访问密钥无效")
		return
	}
	id := sha256.Sum256([]byte(secret))
	g.handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gatewayCallerKey{}, hex.EncodeToString(id[:]))))
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

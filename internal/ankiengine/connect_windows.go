package ankiengine

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
)

type ConnectSettings struct {
	Enabled bool     `json:"enabled"`
	Port    int      `json:"port"`
	Origins []string `json:"origins"`
}
type ConnectState struct {
	ConnectSettings
	Running bool   `json:"running"`
	Key     string `json:"key"`
	Error   string `json:"error,omitempty"`
}
type Connect struct {
	mu        sync.Mutex
	engine    *Engine
	path      string
	settings  ConnectSettings
	key       string
	server    *http.Server
	errorText string
	onEvents  func([]json.RawMessage) error
}

func NewConnect(engine *Engine, directory string, onEvents func([]json.RawMessage) error) (*Connect, error) {
	c := &Connect{engine: engine, path: filepath.Join(directory, "anki-connect.json"), settings: ConnectSettings{Port: 8765, Origins: []string{}}, onEvents: onEvents}
	if data, err := os.ReadFile(c.path); err == nil {
		if err = json.Unmarshal(data, &c.settings); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if data, err := os.ReadFile(c.path + ".key"); err == nil {
		plain, err := ai.UnprotectLocalData(data)
		if err != nil {
			return nil, err
		}
		c.key = string(plain)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// A previously explicit enablement survives restart; a fresh install never listens.
	if c.settings.Enabled {
		if err := c.configure(c.settings); err != nil {
			c.errorText = err.Error()
		}
	}
	return c, nil
}
func validateConnect(s ConnectSettings) error {
	if s.Port < 1024 || s.Port > 65535 {
		return errors.New("AnkiConnect 端口必须在 1024–65535 之间")
	}
	for _, origin := range s.Origins {
		if origin == "*" || strings.ContainsAny(origin, "\r\n\x00") || !(strings.HasPrefix(origin, "http://") || strings.HasPrefix(origin, "https://") || strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://")) {
			return errors.New("请填写完整的允许来源，不支持通配符")
		}
	}
	return nil
}
func (c *Connect) State() ConnectState {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.settings
	s.Origins = append([]string{}, s.Origins...)
	return ConnectState{ConnectSettings: s, Running: c.server != nil, Key: c.key, Error: c.errorText}
}
func (c *Connect) Configure(s ConnectSettings) (ConnectState, error) {
	c.mu.Lock()
	err := c.configure(s)
	c.mu.Unlock()
	return c.State(), err
}
func (c *Connect) configure(s ConnectSettings) error {
	if err := validateConnect(s); err != nil {
		return err
	}
	if c.key == "" {
		key := rand.Text() + rand.Text()
		encrypted, err := ai.ProtectLocalData([]byte(key))
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(c.path+".key", encrypted, 0600); err != nil {
			return err
		}
		c.key = key
	}
	var listener net.Listener
	if s.Enabled && (c.server == nil || c.settings.Port != s.Port) {
		var err error
		listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)))
		if err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		if listener != nil {
			listener.Close()
		}
		return err
	}
	if err = os.WriteFile(c.path+".tmp", data, 0600); err == nil {
		err = os.Rename(c.path+".tmp", c.path)
	}
	if err != nil {
		if listener != nil {
			listener.Close()
		}
		return err
	}
	if c.server != nil && (!s.Enabled || c.settings.Port != s.Port) {
		_ = c.server.Close()
		c.server = nil
	}
	c.settings = s
	c.errorText = ""
	if listener != nil {
		server := &http.Server{Handler: c, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 155 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 32768}
		c.server = server
		go func() {
			err := server.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				c.mu.Lock()
				if c.server == server {
					c.errorText = err.Error()
					c.server = nil
				}
				c.mu.Unlock()
			}
		}()
	}
	return nil
}
func (c *Connect) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server == nil {
		return nil
	}
	err := c.server.Close()
	c.server = nil
	return err
}
func (c *Connect) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	key := c.key
	origins := append([]string{}, c.settings.Origins...)
	c.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/" && r.URL.Path != "/api/v1" {
		http.NotFound(w, r)
		return
	}
	origin := r.Header.Get("Origin")
	allowed := origin == ""
	for _, value := range origins {
		allowed = allowed || origin == value
	}
	if !allowed {
		http.Error(w, "Origin is not allowed", http.StatusForbidden)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Use POST", http.StatusMethodNotAllowed)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 96<<20))
	if err != nil {
		http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
		return
	}
	var request map[string]json.RawMessage
	if !json.Valid(data) {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.URL.Path == "/" {
		if err = json.Unmarshal(data, &request); err != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		_ = json.Unmarshal(request["key"], &provided)
	}
	if key == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
		http.Error(w, "A valid API key is required", http.StatusUnauthorized)
		return
	}
	method := "winone.legacy"
	if r.URL.Path == "/api/v1" {
		method = "winone.rpc"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	var result json.RawMessage
	events, err := c.engine.Call(ctx, method, map[string]any{"request": json.RawMessage(data)}, &result)
	if c.onEvents != nil && len(events) > 0 {
		if eventErr := c.onEvents(events); eventErr != nil {
			err = errors.Join(err, eventErr)
		}
	}
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"result": nil, "error": err.Error()})
		return
	}
	if len(result) == 0 || string(result) == "null" && r.URL.Path == "/api/v1" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = w.Write(result)
}

package chat

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PrivateBridge exists only for a live Pi worker. Its port and token are never
// persisted or exposed through the desktop API or the public gateway settings.
type PrivateBridge struct {
	BaseURL string
	Key     string
	server  *http.Server
	cancel  context.CancelFunc
	once    sync.Once
}

func NewPrivateBridge(parent context.Context, handler http.Handler) (*PrivateBridge, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	b := &PrivateBridge{BaseURL: "http://" + listener.Addr().String() + "/v1", Key: rand.Text() + rand.Text(), cancel: cancel}
	b.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "127.0.0.1" || r.Header.Get("Origin") != "" {
			http.Error(w, "Local native clients only", http.StatusForbidden)
			return
		}
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(key), []byte(b.Key)) != 1 {
			http.Error(w, "Invalid session token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})}
	go func() { defer cancel(); _ = b.server.Serve(listener) }()
	go func() { <-ctx.Done(); b.Close() }()
	return b, nil
}

// Cancel in-flight model requests before closing connections; Pi is closed by
// its owner before the bridge, so a normal exit can finish its session writes.
func (b *PrivateBridge) Close() { b.once.Do(func() { b.cancel(); _ = b.server.Close() }) }

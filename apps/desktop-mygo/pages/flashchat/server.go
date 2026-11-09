package flashchat

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The production build creates these assets from the pinned full upstream.
//
//go:embed all:assets
var bundled embed.FS

type localApp struct {
	server *http.Server
	origin string
}

// A stable loopback origin keeps IndexedDB identity/settings across launches.
// A real HTTP listener also supports StreamSaver service workers on WebView2.
// Never attach to another process when the saved port is occupied.
func startLocalApp(directory string, assets fs.FS) (*localApp, error) {
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return nil, errors.New("Flash Chat assets are missing; run build/build-flashchat.mjs")
	}
	configDir := filepath.Join(directory, "flashchat")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	portFile := filepath.Join(configDir, "port")
	port := 0
	data, err := os.ReadFile(portFile)
	if err == nil {
		port, err = strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || port < 1024 || port > 65535 {
			return nil, errors.New("Invalid saved Flash Chat port")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("Flash Chat local address unavailable: %w", err)
	}
	if port == 0 {
		port = listener.Addr().(*net.TCPAddr).Port
		if err := os.WriteFile(portFile, []byte(strconv.Itoa(port)), 0600); err != nil {
			listener.Close()
			return nil, err
		}
	}
	app := &localApp{origin: "http://" + listener.Addr().String()}
	app.server = &http.Server{Handler: assetHandler(assets, listener.Addr().String()), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go app.server.Serve(listener)
	return app, nil
}

// Only packaged resources are served. Unknown asset paths fail instead of
// returning HTML; known SPA routes can be refreshed without a remote server.
func assetHandler(assets fs.FS, host string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.Error(w, "Forbidden", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'; object-src 'none'; base-uri 'self'")
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "about" || name == "settings" || name == "disclaimer" || strings.HasPrefix(name, "private/") || strings.HasPrefix(name, "public/") {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

func (a *localApp) close() {
	if a != nil {
		_ = a.server.Close()
	}
}

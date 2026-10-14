package chat

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type previewGrant struct {
	id, path, target string
	until            time.Time
}
type PreviewServer struct {
	base   string
	server *http.Server
	store  *Store
	mu     sync.Mutex
	grants map[string]previewGrant
}

// A read-only, capability URL serves media with native HTTP range requests.
// It exposes opened files and an HTML document's local assets, never a listing.
func NewPreviewServer(ctx context.Context, store *Store) (*PreviewServer, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &PreviewServer{base: "http://" + listener.Addr().String(), store: store, grants: map[string]previewGrant{}}
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = s.server.Serve(listener) }()
	go func() { <-ctx.Done(); _ = s.Close() }()
	return s, nil
}
func (s *PreviewServer) Close() error { return s.server.Close() }
func (s *PreviewServer) Open(id, path string) (FilePreview, error) {
	p, err := s.store.PreviewFile(id, path)
	if err != nil {
		return p, err
	}
	target := p.Path
	if p.ResolvedPath != "" {
		target = p.ResolvedPath
	}
	key := rand.Text()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, grant := range s.grants {
		if time.Now().After(grant.until) {
			delete(s.grants, key)
		}
	}
	s.grants[key] = previewGrant{id, p.Path, target, time.Now().Add(time.Hour)}
	p.URL = s.base + "/" + key + "/" + url.PathEscape(p.Name)
	return p, nil
}
func (s *PreviewServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "Read only", http.StatusMethodNotAllowed)
		return
	}
	if "http://"+r.Host != s.base {
		http.Error(w, "Invalid host", http.StatusForbidden)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	grant, ok := s.grants[parts[0]]
	s.mu.Unlock()
	if !ok || time.Now().After(grant.until) {
		http.NotFound(w, r)
		return
	}
	path, err := s.store.ResolvePreviewFile(grant.id, grant.path)
	if err != nil || !strings.EqualFold(path, grant.target) {
		http.Error(w, "File unavailable", http.StatusForbidden)
		return
	}
	requested := filepath.FromSlash(parts[1])
	if requested != filepath.Base(grant.path) {
		// Local HTML/CSS/module assets share the document capability, constrained
		// to its own directory and the original real directory after link resolution.
		ext := strings.ToLower(filepath.Ext(grant.path))
		if (ext != ".html" && ext != ".htm" && ext != ".xhtml") || filepath.IsAbs(requested) || strings.Contains(requested, ":") {
			http.NotFound(w, r)
			return
		}
		alias := filepath.Join(filepath.Dir(grant.path), requested)
		if !within(filepath.Dir(grant.path), alias) {
			http.Error(w, "Asset outside document folder", http.StatusForbidden)
			return
		}
		path, err = s.store.ResolvePreviewFile(grant.id, alias)
		if err != nil || !within(filepath.Dir(grant.target), path) {
			http.Error(w, "Asset unavailable", http.StatusForbidden)
			return
		}
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "Read failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// HTML is an isolated preview: no native bridge, forms, external fetches or
	// top-level navigation. The real browser panel owns unrestricted web browsing.
	// Applying sandbox to every response also disables Chromium's built-in PDF
	// viewer. Only active HTML documents need the isolated execution policy.
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".svg", ".xhtml":
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; form-action 'none'; base-uri 'none'")
	}
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

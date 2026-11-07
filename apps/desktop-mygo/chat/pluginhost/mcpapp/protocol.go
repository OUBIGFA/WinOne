package mcpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

var mounted = struct {
	sync.Mutex
	pages map[string]*Page
}{pages: map[string]*Page{}}
var protocolErr = mygo.Protocol.HandleFunc("cindy-mcp-app", serve)
var nativeHTTP = &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func browserOrigin(id string, resource bool) string {
	if resource {
		id += "-view"
	}
	return "http://" + id + ".cindy-mcp-app.localhost"
}
func (p *Page) authorizedPage(request *http.Request) bool {
	if p.disposed.Load() {
		return false
	}
	p.mu.Lock()
	view, session := p.view, p.session
	p.mu.Unlock()
	if view == nil || view.IsDestroyed() || mygo.CallerPage(request.Context()) != view.Page() {
		return false
	}
	address, err := url.Parse(view.Page().URL())
	if err != nil {
		return false
	}
	if sameProxyDocument(session.ViewID, address) {
		return true
	}
	// WebResourceRequested can arrive before Source commits the first URL.
	// Only this one initial Host-issued document is allowed while still blank.
	return address.String() == "about:blank" && request.Method == "GET" && request.URL.Host == session.ViewID && request.URL.Path == "/sandbox" && p.proxyServed.CompareAndSwap(false, true)
}

// Browser Origin is forbidden to scripts; CallerPage is set by native MyGo,
// not by request headers. Only the trusted proxy can acquire the RPC channel.
func allowedRequest(id string, request *http.Request) (string, bool) {
	resource := request.URL.Host == id+"-view"
	if !resource {
		if request.URL.Path == "/sandbox" && request.Method == "GET" {
			return "sandbox", true
		}
		if (request.URL.Path == "/bridge" || request.URL.Path == "/state" || request.URL.Path == "/environment") && request.Method == "POST" && request.Header.Get("Origin") == browserOrigin(id, false) {
			return strings.TrimPrefix(request.URL.Path, "/"), true
		}
		return "", false
	}
	if request.Method != "GET" && request.Method != "HEAD" {
		return "", false
	}
	switch request.URL.Path {
	case "/resource":
		return "resource", request.Method == "GET"
	case "/__cindy/sdk/ext-apps/app.js":
		return "sdk", request.Method == "GET"
	}
	if strings.HasPrefix(request.URL.Path, "/__cindy/files/") {
		token := strings.TrimPrefix(request.URL.Path, "/__cindy/files/")
		if !viewID.MatchString(token) {
			return "", false
		}
		// A per-View token carries only this card's links. Cross-origin browser
		// requests are denied; ordinary same-origin GETs need not set Origin.
		origin := request.Header.Get("Origin")
		if origin != "" && origin != browserOrigin(id, true) {
			return "", false
		}
		if ref := request.Header.Get("Referer"); ref != "" {
			u, err := url.Parse(ref)
			if err != nil || u.Scheme+"://"+u.Host != browserOrigin(id, true) || u.Path != "/resource" {
				return "", false
			}
		}
		return "files/" + token, true
	}
	return "", false
}

func serve(writer http.ResponseWriter, request *http.Request) {
	id, valid := requestView(request.URL)
	if !valid {
		http.NotFound(writer, request)
		return
	}
	mounted.Lock()
	page := mounted.pages[id]
	mounted.Unlock()
	if page == nil || !page.authorizedPage(request) {
		http.NotFound(writer, request)
		return
	}
	route, allowed := allowedRequest(id, request)
	if !allowed {
		http.Error(writer, "MCP_APP_FRAME_REQUIRED", http.StatusForbidden)
		return
	}
	page.mu.Lock()
	session := page.session
	page.mu.Unlock()
	var body io.Reader
	if request.Method == "POST" {
		raw, err := io.ReadAll(io.LimitReader(request.Body, 4<<20+1))
		if err != nil || len(raw) > 4<<20 || !json.Valid(raw) {
			http.Error(writer, "MCP_APP_INVALID_REQUEST", http.StatusBadRequest)
			return
		}
		body = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(page.ctx, cancel)
	defer func() { stop(); cancel() }()
	upstream, err := http.NewRequestWithContext(ctx, request.Method, session.Bridge.BaseURL+"/"+route, body)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	// The bearer is native-only and unique to this View. It never appears in
	// HTML, the official App runtime, Javascript requests or model/history data.
	upstream.Header.Set("Authorization", "Bearer "+session.Bridge.Key)
	if body != nil {
		upstream.Header.Set("Content-Type", "application/json")
	}
	if value := request.Header.Get("Range"); value != "" {
		upstream.Header.Set("Range", value)
	}
	response, err := nativeHTTP.Do(upstream)
	if err != nil {
		http.Error(writer, "MCP_APP_VIEW_UNAVAILABLE", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if route == "state" && response.StatusCode == http.StatusOK {
		var state UIState
		if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&state) != nil {
			http.Error(writer, "MCP_APP_INVALID_STATE", http.StatusBadGateway)
			return
		}
		page.receiveState(state)
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{}`))
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Security-Policy", "Permissions-Policy", "Accept-Ranges", "Content-Range"} {
		if value := response.Header.Get(key); value != "" {
			writer.Header().Set(key, value)
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(response.StatusCode)
	// The original Host Range response streams, rather than becoming a 64 MiB
	// base64 JSON result. Closing the native page cancels the same request.
	_, _ = io.Copy(writer, response.Body)
}

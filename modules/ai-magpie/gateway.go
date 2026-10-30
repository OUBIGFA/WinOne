package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

type gatewayEvent struct {
	Limited bool        `json:"limited,omitempty"`
	Type    string      `json:"type"`
	Status  int         `json:"status,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Data    []byte      `json:"data,omitempty"`
}
type gatewayResponse struct {
	errorBody []byte
	header    http.Header
	status    int
	encode    func(gatewayEvent) error
	err       error
}

func (w *gatewayResponse) Header() http.Header { return w.header }
func (w *gatewayResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	// An error's header waits until its bounded body can be classified. A
	// successful response still starts streaming immediately.
	if status >= 400 {
		return
	}
	w.err = w.encode(gatewayEvent{Type: "headers", Status: status, Headers: w.header})
}
func (w *gatewayResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if w.err != nil {
		return 0, w.err
	}
	// Upstream errors may repeat a provider key or a prompt. Only status and
	// Retry-After leave this worker; successful protocol bodies stay byte-exact.
	if w.status >= 400 {
		w.errorBody = appendFailureBody(w.errorBody, data)
		return len(data), nil
	}
	total := len(data)
	for len(data) > 0 {
		n := min(len(data), 32<<10)
		if err := w.encode(gatewayEvent{Type: "data", Data: data[:n]}); err != nil {
			w.err = err
			return 0, err
		}
		data = data[n:]
	}
	return total, nil
}
func (w *gatewayResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
}

// Keep the entire client's protocol payload, including tools and images.
// Magpie, rather than a second translator, owns all cross-protocol conversion.
func serveGateway(ctx context.Context, in request, output io.Writer) error {
	stopPlugins := serveBuiltinPlugins()
	defer stopPlugins()
	if err := configureUsage(in); err != nil {
		return err
	}
	global := in.Operation == "magpie.gateway"
	var payload map[string]json.RawMessage
	if global {
		provider.WinOneProviders = []provider.Provider{}
		if err := installAuxiliaryProviders(in); err != nil {
			return err
		}
		return serveGatewayBody(ctx, in, in.GatewayBody, output)
	}
	if json.Unmarshal(in.GatewayBody, &payload) != nil || payload == nil {
		return errors.New("invalid gateway body")
	}
	model := "winone/" + in.Model
	if in.Agent != "" {
		p, err := findAccount(ctx, in.Agent)
		if err != nil {
			return err
		}
		model = p.ID + "/" + in.Model
		p.LimitRestOff = in.LimitRestOff
		provider.WinOneProviders = []provider.Provider{*p}
	} else {
		p, err := configuredProvider(in)
		if err != nil {
			return err
		}
		provider.WinOneProviders = []provider.Provider{p}
	}
	payload["model"], _ = json.Marshal(model)
	if err := installModelPreferences(strings.TrimSuffix(model, "/"+in.Model), in.Model, in.ModelMetadata); err != nil {
		return err
	}
	if err := installAuxiliaryProviders(in); err != nil {
		return err
	}
	metadata := in.ModelMetadata
	if gateway.WinOneTransportOnly {
		// The outer router already chose the effort/tier. A provider transport
		// must retain that decision instead of applying its saved defaults again.
		metadata.ReasoningEffort, metadata.Tier, metadata.Reasoning = "", "", nil
	}
	if err := applyRawModelMetadata(payload, in.GatewayPath, metadata); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return serveGatewayBody(ctx, in, body, output)
}

func serveGatewayBody(ctx context.Context, in request, body []byte, output io.Writer) error {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+in.GatewayPath, bytes.NewReader(body)).WithContext(ctx)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gateway.Token)
	for _, name := range []string{"Anthropic-Version", "Anthropic-Beta", "Openai-Beta", "MCP-Protocol-Version", "Accept", "User-Agent", "Originator", "X-OpenAI-Subagent"} {
		if value := in.GatewayHeaders.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	encoder := json.NewEncoder(output)
	response := &gatewayResponse{header: make(http.Header), encode: func(event gatewayEvent) error { return encoder.Encode(event) }}
	if id := in.GatewayHeaders.Get("X-WinOne-Caller-Key"); id != "" {
		req.Header.Set("X-WinOne-Caller-Key", id)
	}
	gateway.New().Handler().ServeHTTP(response, winOneCallerRequest(req))
	gateway.WinOneWaitArchive()
	if response.status == 0 {
		response.WriteHeader(200)
	}
	if response.err != nil {
		return response.err
	}
	if response.status >= 400 {
		if err := encoder.Encode(gatewayEvent{Type: "headers", Status: response.status, Headers: response.header, Limited: gateway.WinOneLimitedResponse(response.status, response.errorBody)}); err != nil {
			return err
		}
	}
	return encoder.Encode(gatewayEvent{Type: "done"})
}

// The private Host transport authenticates before supplying this public hash.
// Reuse upstream caller accounting without registering or copying real keys.
func winOneCallerRequest(r *http.Request) *http.Request {
	id := r.Header.Get("X-WinOne-Caller-Key")
	r.Header.Del("X-WinOne-Caller-Key")
	if len(id) == 64 {
		if _, err := hex.DecodeString(id); err == nil {
			return r.WithContext(access.WithIdentity(r.Context(), access.Identity{KeyID: id, KeyName: "WinOne gateway"}))
		}
	}
	return r
}

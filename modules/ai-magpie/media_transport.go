package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"net/http"
	"strings"
)

// The authenticated outer gateway has already validated model policy, privacy,
// keys and video ownership. A transport-only inner worker must not wrap video
// ids/models again. Reuse the upstream provider's Sign/Do (headers, proxy, TLS,
// plugin accounts), while limiting this seam to the existing media API paths.
func serveMediaTransport(ctx context.Context, in request, output io.Writer) (bool, error) {
	if in.Operation != "gateway" {
		return false, nil
	}
	path := in.GatewayPath
	method := in.GatewayMethod
	if method == "" {
		method = http.MethodPost
	}
	allowed := method == http.MethodPost && (path == "/v1/images/generations" || path == "/v1/images/edits" || path == "/v1/videos" || path == "/v1/embeddings")
	allowed = allowed || method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/") && !strings.ContainsAny(strings.TrimPrefix(path, "/v1/videos/"), "?\\\x00")
	if !allowed {
		return false, nil
	}
	var selected provider.Provider
	if in.Agent != "" {
		account, err := findAccount(ctx, in.Agent)
		if err != nil {
			return true, err
		}
		selected = *account
	} else {
		p, err := configuredProvider(in)
		if err != nil {
			return true, err
		}
		selected = p
	}
	base := selected.Base(provider.Chat)
	if base == "" {
		base = selected.Base(provider.Responses)
	}
	if base == "" {
		return true, errors.New("media provider endpoint unavailable")
	}
	body := []byte(in.GatewayBody)
	if len(in.GatewayBinaryBody) > 0 {
		body = in.GatewayBinaryBody
	}
	if method == http.MethodGet {
		body = nil
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+"/"+strings.TrimPrefix(path, "/v1/"), bytes.NewReader(body))
	if err != nil {
		return true, err
	}
	contentType := in.GatewayContentType
	if contentType == "" {
		contentType = "application/json"
	}
	if body != nil {
		request.Header.Set("Content-Type", contentType)
	}
	if err = selected.Sign(ctx, request, provider.Chat, body); err != nil {
		return true, err
	}
	response, err := selected.Do(http.DefaultClient, request)
	if err != nil {
		return true, err
	}
	defer response.Body.Close()
	encoder := json.NewEncoder(output)
	stream := &gatewayResponse{header: response.Header.Clone(), encode: func(event gatewayEvent) error { return encoder.Encode(event) }}
	stream.WriteHeader(response.StatusCode)
	if _, err = io.Copy(stream, response.Body); err != nil {
		return true, err
	}
	if response.StatusCode >= 400 {
		if err = encoder.Encode(gatewayEvent{Type: "headers", Status: response.StatusCode, Headers: stream.header}); err != nil {
			return true, err
		}
	}
	return true, encoder.Encode(gatewayEvent{Type: "done"})
}

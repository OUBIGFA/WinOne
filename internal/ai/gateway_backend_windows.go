//go:build windows

package ai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GatewayProcess owns only the local gateway and its private administration.
// Its Job Object and pipes preserve the normal WinOne process ownership.
type GatewayProcess struct {
	updateCatalog func(Provider)
	URL, Key      string
	process       *aiProcess
	stopAuxiliary func()
	once          sync.Once
}

func (r *GatewayProcess) Close() {
	if r == nil {
		return
	}
	r.once.Do(func() { r.process.input.Close(); r.process.closeJob(); <-r.process.finished; r.stopAuxiliary() })
}

// A terminated worker is never reused by the Host's cached gateway session.
func (r *GatewayProcess) Alive() bool {
	if r == nil {
		return false
	}
	select {
	case <-r.process.finished:
		return false
	default:
		return true
	}
}

// AdminURL is private to the native settings window; the public gateway never
// forwards management paths or this capability.
func (r *GatewayProcess) AdminURL() string {
	return r.URL + "/session/" + url.PathEscape(r.Key) + "/"
}

// SyncCatalog preserves in-flight gateway calls and the call history.
// Only a provider's model list/metadata change; credentials stay in its bridge.
func (r *GatewayProcess) SyncCatalog(ctx context.Context, p Provider) error {
	// Install transport support before publishing IDs to the outer gateway.
	// Its private bridge retains old models for calls already in flight.
	if r.updateCatalog != nil {
		r.updateCatalog(p)
	}
	raw, _ := json.Marshal(map[string]any{"id": p.ID, "models": p.Models, "metadata": p.ModelMetadata, "contexts": p.Contexts, "modelNames": p.ModelNames})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/api/winone/catalog", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("模型目录未能同步到网关服务")
	}
	return nil
}

type mediaModelsKey struct{}

// Carry the Host's current model policy and public listener address into
// Magpie. These values never come from a client's lookalike headers.
func (r *GatewayProcess) ServeMedia(w http.ResponseWriter, request *http.Request, models []string) {
	r.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), mediaModelsKey{}, models)))
}

func (r *GatewayProcess) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	target, err := url.Parse(r.URL + request.URL.RequestURI())
	if err != nil {
		GatewayError(w, 502, "网关地址无效")
		return
	}
	cloned := request.Clone(request.Context())
	cloned.URL = target
	cloned.RequestURI = ""
	cloned.Host = target.Host
	cloned.Header = cloned.Header.Clone()
	cloned.Header.Del("X-WinOne-Product")
	cloned.Header.Del("X-WinOne-Session")

	cloned.Header.Del("X-WinOne-Media-Models")
	if models, ok := request.Context().Value(mediaModelsKey{}).([]string); ok {
		raw, _ := json.Marshal(models)
		cloned.Header.Set("X-WinOne-Media-Models", string(raw))
		cloned.Host = request.Host
	}
	// Discard an incoming lookalike; this metadata belongs to Host authentication.
	cloned.Header.Del("X-WinOne-Caller-Key")
	if id := GatewayCallerKeyID(request.Context()); id != "" {
		cloned.Header.Set("X-WinOne-Caller-Key", id)
	}
	cloned.Header.Set("Authorization", "Bearer "+r.Key)
	cloned.Header.Del("X-WinOne-Gateway-Credential")
	cloned.Header.Del("X-WinOne-Remote-Addr")
	if key := GatewayCredential(request.Context()); key != "" {
		cloned.Header.Set("X-WinOne-Gateway-Credential", key)
		cloned.Header.Set("X-WinOne-Remote-Addr", request.RemoteAddr)
	}
	if strings.EqualFold(request.Header.Get("Upgrade"), "websocket") {
		// The standard proxy preserves the duplex upgrade and cancellation.
		// The Host model policy and private credential were attached above.
		proxy := httputil.ReverseProxy{Director: func(out *http.Request) {
			out.URL = cloned.URL
			out.Host = cloned.Host
			out.Header = cloned.Header.Clone()
		}, ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) { GatewayError(w, 502, "网关连接失败") }}
		proxy.ServeHTTP(w, request)
		return
	}
	response, err := http.DefaultClient.Do(cloned)
	if err != nil {
		GatewayError(w, 502, "网关服务不可用")
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	buffer := make([]byte, 32<<10)
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			if _, failure := w.Write(buffer[:n]); failure != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
func (t Translator) StartGateway(ctx context.Context, prefs *MagpieSettings, providers []RuntimeProvider) (*GatewayProcess, error) {
	// Gateway workers share pipe authentication and Job ownership with the
	// short-lived provider operations.

	var updateCatalog func(Provider)
	auxiliary, stop, err := t.auxiliaryBridgeCatalog(ctx, providers, prefs, &updateCatalog)
	if err != nil {
		return nil, err
	}
	process, err := t.start(ctx)
	if err != nil {
		stop()
		return nil, err
	}
	key := rand.Text() + rand.Text()
	router := &GatewayProcess{process: process, stopAuxiliary: stop, Key: key, updateCatalog: updateCatalog}
	started := make(chan []byte, 1)
	// One reader owns stdout and reaps the process even after an unexpected
	// exit. Closing its job releases a blocked scan during startup cancellation.
	go func() {
		scanner := bufio.NewScanner(process.output)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		if scanner.Scan() {
			started <- bytes.Clone(scanner.Bytes())
		} else {
			started <- nil
		}
		io.Copy(io.Discard, process.output)
		_ = process.finish()
	}()
	if err = json.NewEncoder(process.input).Encode(map[string]any{"operation": "gateway.serve", "key": key, "magpieSettings": prefs, "auxiliaryProviders": auxiliary}); err != nil {
		router.Close()
		return nil, err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	var initial []byte
	select {
	case initial = <-started:
	case <-ctx.Done():
		router.Close()
		return nil, ctx.Err()
	case <-timer.C:
		router.Close()
		return nil, errors.New("网关服务启动超时")
	}
	if len(initial) == 0 {
		router.Close()
		return nil, errors.New("网关服务未能启动")
	}
	var result struct{ URL, Error string }
	if json.Unmarshal(initial, &result) != nil || !strings.HasPrefix(result.URL, "http://127.0.0.1:") {
		router.Close()
		return nil, errors.New("网关服务返回无效地址")
	}
	router.URL = result.URL
	return router, nil
}

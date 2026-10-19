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
	"net/url"
	"strings"
	"sync"
	"time"
)

// RoutingProcess shares one upstream router while it is enabled or visible.
// Its Job Object and pipes preserve the normal WinOne process ownership.
type RoutingProcess struct {
	URL, Key      string
	process       *aiProcess
	stopAuxiliary func()
	once          sync.Once
}

func (r *RoutingProcess) Close() {
	if r == nil {
		return
	}
	r.once.Do(func() { r.process.input.Close(); r.process.closeJob(); <-r.process.finished; r.stopAuxiliary() })
}

// A terminated worker is never reused by the Host's cached routing session.
func (r *RoutingProcess) Alive() bool {
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
func (r *RoutingProcess) PageURL() string {
	return r.URL + "/session/" + url.PathEscape(r.Key) + "/?view=routing"
}

func (r *RoutingProcess) PluginsPageURL() string {
	return r.URL + "/session/" + url.PathEscape(r.Key) + "/plugins.html"
}

// PluginWindowState contains only the upstream's public page/panel manifests.
// Their reads and options remain behind the same authenticated runtime, not
// arbitrary URLs supplied by the settings renderer.
type PluginWindowState struct {
	URL    string          `json:"url"`
	Pages  json.RawMessage `json:"pages"`
	Panels json.RawMessage `json:"panels"`
}

func (r *RoutingProcess) PluginWindowState(ctx context.Context) (PluginWindowState, error) {
	state := PluginWindowState{URL: r.PluginsPageURL()}
	if err := r.readAPI(ctx, "/api/plugin-pages", &state.Pages); err != nil {
		return state, err
	}
	if err := r.readAPI(ctx, "/api/plugin-panels", &state.Panels); err != nil {
		return state, err
	}
	return state, nil
}

type routingGroup struct {
	ID         string   `json:"id"`
	Members    []string `json:"members"`
	Matched    []string `json:"matched"`
	Off        []string `json:"off"`
	Classifier string   `json:"classifier"`
	Disabled   bool     `json:"disabled"`
	Ready      bool     `json:"ready"`
}

func (r *RoutingProcess) readAPI(ctx context.Context, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+r.Key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("路由状态读取失败")
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(out)
}
func (r *RoutingProcess) modelFilter(ctx context.Context, allow func(Selection) bool) (func(string) bool, error) {
	var state struct {
		Groups []routingGroup `json:"groups"`
	}
	if err := r.readAPI(ctx, "/api/groups", &state); err != nil {
		return nil, err
	}
	groups := map[string]routingGroup{}
	for _, group := range state.Groups {
		groups[group.ID] = group
	}
	var permitted func(string, map[string]bool) bool
	permitted = func(ref string, seen map[string]bool) bool {
		provider, model, ok := strings.Cut(ref, "/")
		if !ok {
			return false
		}
		if provider != "group" {
			if allow(Selection{Provider: provider, Model: model}) {
				return true
			}
			// A fixed effort is appended to a member ID by Magpie. Vendor IDs
			// such as :free keep their complete spelling and are never shortened.
			base, effort, found := strings.Cut(model, ":")
			if !found {
				return false
			}
			switch effort {
			case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
				return allow(Selection{Provider: provider, Model: base})
			default:
				return false
			}
		}
		group, found := groups[model]
		if !found || group.Disabled || !group.Ready || seen[model] {
			return false
		}
		seen[model] = true
		defer delete(seen, model)
		members := append(append([]string{}, group.Members...), group.Matched...)
		count := 0
		for _, member := range members {
			off := false
			for _, value := range group.Off {
				off = off || value == member
			}
			if off {
				continue
			}
			count++
			if !permitted(member, seen) {
				return false
			}
		}
		return count > 0 && (group.Classifier == "" || permitted(group.Classifier, seen))
	}
	return func(ref string) bool { return permitted(ref, map[string]bool{}) }, nil
}

// A public gateway's per-model selection remains authoritative for groups too.
// A group cannot bypass a disabled member or classifier through the router.
func (r *RoutingProcess) AllowsModel(ctx context.Context, model string, allow func(Selection) bool) (bool, error) {
	filter, err := r.modelFilter(ctx, allow)
	if err != nil {
		return false, err
	}
	return filter(model), nil
}
func (r *RoutingProcess) Models(ctx context.Context, allow func(Selection) bool) (json.RawMessage, error) {
	filter, err := r.modelFilter(ctx, allow)
	if err != nil {
		return nil, err
	}
	var list struct {
		Data []json.RawMessage `json:"data"`
	}
	if err = r.readAPI(ctx, "/v1/models", &list); err != nil {
		return nil, err
	}
	models := []json.RawMessage{}
	for _, raw := range list.Data {
		var model struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &model) != nil {
			return nil, errors.New("路由模型列表无效")
		}
		if filter(model.ID) {
			models = append(models, raw)
		}
	}
	return json.Marshal(map[string]any{"object": "list", "data": models})
}
func (r *RoutingProcess) ServeBody(w http.ResponseWriter, request *http.Request, path string, body []byte) {
	cloned := request.Clone(request.Context())
	target := *request.URL
	cloned.URL = &target
	cloned.URL.Path = path
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.RequestURI = path
	r.ServeHTTP(w, cloned)
}
func (r *RoutingProcess) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	target, err := url.Parse(r.URL + request.URL.RequestURI())
	if err != nil {
		GatewayError(w, 502, "路由地址无效")
		return
	}
	cloned := request.Clone(request.Context())
	cloned.URL = target
	cloned.RequestURI = ""
	cloned.Host = target.Host
	cloned.Header = cloned.Header.Clone()
	// Discard an incoming lookalike; this metadata belongs to Host authentication.
	cloned.Header.Del("X-WinOne-Caller-Key")
	if id := GatewayCallerKeyID(request.Context()); id != "" {
		cloned.Header.Set("X-WinOne-Caller-Key", id)
	}
	cloned.Header.Set("Authorization", "Bearer "+r.Key)
	response, err := http.DefaultClient.Do(cloned)
	if err != nil {
		GatewayError(w, 502, "路由服务不可用")
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
func (t Translator) StartRouting(ctx context.Context, prefs *MagpieSettings, providers []RuntimeProvider) (*RoutingProcess, error) {
	auxiliary, stop, err := t.auxiliaryBridge(ctx, providers, prefs)
	if err != nil {
		return nil, err
	}
	process, err := t.start(ctx)
	if err != nil {
		stop()
		return nil, err
	}
	key := rand.Text() + rand.Text()
	router := &RoutingProcess{process: process, stopAuxiliary: stop, Key: key}
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
	if err = json.NewEncoder(process.input).Encode(map[string]any{"operation": "routing.serve", "key": key, "magpieSettings": prefs, "auxiliaryProviders": auxiliary}); err != nil {
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
		return nil, errors.New("路由服务启动超时")
	}
	if len(initial) == 0 {
		router.Close()
		return nil, errors.New("路由服务未能启动")
	}
	var result struct{ URL, Error string }
	if json.Unmarshal(initial, &result) != nil || !strings.HasPrefix(result.URL, "http://127.0.0.1:") {
		router.Close()
		return nil, errors.New("路由服务返回无效地址")
	}
	router.URL = result.URL
	return router, nil
}

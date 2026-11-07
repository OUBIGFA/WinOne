package aiservices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	"winone/internal/ai"
)

// This test owns the real Magpie process, profile and vendor fixture. Native
// controls issue production HTTP requests, not a copied routing implementation.
func nativeAIWithRuntime(t *testing.T) (*page, *testhost.Harness, *ui.Tester, *ai.RoutingProcess) {
	t.Helper()
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated Magpie runtime")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Native routed answer"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	settings := &ai.MagpieSettings{NoStats: true, NoUsageStats: true, NoAccountDiscovery: true, Redact: true, RedactWords: []string{"fixture-private-password"}}
	router, err := (ai.Translator{Executable: exe, DataDirectory: filepath.Join(t.TempDir(), "runtime")}).StartRouting(ctx, settings, []ai.RuntimeProvider{{Provider: ai.Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model"}}, Key: "fixture-key"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.Close)
	f := newFakeAI(t)
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "capture.ai.routing.extensions.open" {
			return router.PluginWindowState(ctx)
		}
		return f.handle(method, params)
	})
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	tt := ui.NewTester(p.View, 1100, 1600)
	settle(h, tt)
	return p, h, tt, router
}

func TestNativeRoutingRealCreateReadSaveAndNoWebView(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	p, h, tt, router := nativeAIWithRuntime(t)
	p.switchTab(tabRouting)
	settle(h, tt)
	if p.routing.state == nil {
		t.Fatal("real group read:", p.routing.api.err)
	}
	if err := tt.Click(nativeT("newGroup")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(nativeT("groupName")); err != nil {
		t.Fatal(err)
	}
	tt.Type("Native group")
	if err := tt.Click(nativeT("groupID")); err != nil {
		t.Fatal(err)
	}
	tt.Type("native-group")
	p.routing.edit.Value.Members = []string{"fixture/model"}
	settle(h, tt)
	if err := tt.Click(nativeT("saveGroup")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.routing.api.err != "" || p.routing.edit != nil {
		t.Fatal("real group save:", p.routing.api.err)
	}
	var saved *routeGroup
	for _, doc := range p.routing.state.Groups {
		if doc.Value.ID == "native-group" {
			copy := doc.Value
			saved = &copy
		}
	}
	if saved == nil || len(saved.Members) != 1 {
		t.Fatalf("group missing: %+v", p.routing.state)
	}
	request, err := http.NewRequest("POST", router.URL+"/v1/chat/completions", strings.NewReader(`{"model":"group/native-group","messages":[{"role":"user","content":"check native group"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+router.Key)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		var v any
		json.NewDecoder(response.Body).Decode(&v)
		t.Fatalf("group did not execute: %d %+v", response.StatusCode, v)
	}
	p.routing.tab = 1
	settle(h, tt)
	if len(p.routing.trace.routes) == 0 {
		t.Fatal("real trace missing:", p.routing.trace.api.err)
	}
	route := p.routing.trace.routes[len(p.routing.trace.routes)-1]
	p.routing.trace.detailRead(p, "body", "/api/gateway/call?route="+kitID(route.ID))
	settle(h, tt)
	if p.routing.trace.detailAPI.err != "" || p.routing.trace.detail == nil {
		t.Fatal("route call body:", p.routing.trace.detailAPI.err)
	}
	shot(t, tt, "native-routing-real-trace")
}
func kitID(v int64) string { b, _ := json.Marshal(v); return string(b) }

func TestNativeMaskingRealPreviewAndClear(t *testing.T) {
	p, h, tt, _ := nativeAIWithRuntime(t)
	p.switchTab(tabAdvanced)
	p.advancedTab = advancedPrivacy
	settle(h, tt)
	m := p.advanced.records
	if m == nil || !m.loaded || m.api.err != "" {
		t.Fatalf("records read failed: %+v", m)
	}
	m.previewAPI.request(p, "POST", "/api/masking/preview", map[string]string{"text": "password=fixture-private-password"}, func(raw json.RawMessage) { m.previewResult = raw })
	settle(h, tt)
	if m.previewResult == nil || m.previewAPI.err != "" {
		t.Fatal("masking preview failed:", m.previewAPI.err)
	}
	var preview struct {
		Text string            `json:"text"`
		Hits []json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(m.previewResult, &preview); err != nil || len(preview.Hits) == 0 || strings.Contains(preview.Text, "fixture-private-password") {
		t.Fatalf("preview did not use actual masking: %s", m.previewResult)
	}
	m.api.request(p, "POST", "/api/masking/clear", nil, func(json.RawMessage) { m.loaded = false })
	settle(h, tt)
	if m.api.err != "" {
		t.Fatal("clear failed:", m.api.err)
	}
	shot(t, tt, "native-masking-real")
}

func TestNativeAPIRefusesRedirectAndRetainsMutationFailure(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.Write([]byte(`{}`)) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	_, h, p, tt := setup(t)
	p.extensions.state.URL = server.URL + "/session/private/plugins.html"
	var api nativeAPI
	called := false
	api.request(p, "POST", "/api/groups/save", map[string]string{"id": "private"}, func(json.RawMessage) { called = true })
	settle(h, tt)
	if leaked || called || api.err == "" {
		t.Fatalf("redirect or failed mutation treated as success: leaked=%v called=%v err=%s", leaked, called, api.err)
	}
}

func TestRoutingDraftPreservesUpstreamUnknownFieldsAndRuleTime(t *testing.T) {
	var doc core.Preserved[routeGroup]
	if err := json.Unmarshal([]byte(`{"id":"g","name":"G","members":["p/m"],"futureOption":{"keep":true},"rules":[{"use":"p/m","time":{"from":"09:00","to":"18:00","days":["mon"],"zone":"Asia/Shanghai"},"futureRule":7}]}`), &doc); err != nil {
		t.Fatal(err)
	}
	r := newRoutingPanel(nil)
	r.begin(doc)
	r.edit.Value.Name = "Changed"
	data, _ := json.Marshal(r.edit)
	for _, piece := range []string{`"futureOption"`, `"futureRule":7`, `"from":"09:00"`, `"days":["mon"]`} {
		if !strings.Contains(string(data), piece) {
			t.Fatalf("native form lost %s: %s", piece, data)
		}
	}
}

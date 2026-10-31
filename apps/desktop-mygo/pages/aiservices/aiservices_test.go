package aiservices

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/web"
)

// fakeAI is the Host's AI service as the page sees it: a configuration
// document with fields the page does not declare, a gateway, accounts
// and Magpie's catalog.
type fakeAI struct {
	mu            sync.Mutex
	t             *testing.T
	configuration map[string]any
	saves         []map[string]any
	gateway       map[string]any
	gatewaySaves  []map[string]any
	fetched       []map[string]any
	accounts      []map[string]any
	signIn        []map[string]any // states the sign-in goes through
	signInIndex   int
	keys          map[string]string
	catalog       map[string]any
}

func newFakeAI(t *testing.T) *fakeAI {
	f := &fakeAI{t: t, keys: map[string]string{"one/primary": "sk-saved-123"}}
	f.configuration = map[string]any{
		"revision": 4, "engine": "ai", "ocrEngine": "local", "target": "zh-CN", "prompt": "Translate.",
		"futureSetting": map[string]any{"kept": true},
		"providers": []any{
			map[string]any{"id": "one", "name": "Relay", "authMode": "api-key", "protocol": "chat", "baseUrl": "https://relay.example/v1",
				"models": []any{"gpt-a"}, "availableModels": []any{"gpt-a", "gpt-b"}, "hasKey": true,
				"keys": []any{map[string]any{"id": "primary", "enabled": true, "hasKey": true}},
				"keyPolicy": map[string]any{"strategy": "round-robin", "failover": true, "maxAttempts": 3, "cooldownSeconds": 60},
				"futureProviderField": "kept"},
		},
		"tasks":          map[string]any{"default": map[string]any{"provider": "one", "model": "gpt-a"}, "ocr": map[string]any{"provider": "one", "model": "gpt-b"}},
		"magpieSettings": map[string]any{"quotaLeft": false, "futureMagpie": 7},
	}
	f.gateway = map[string]any{"revision": 2, "enabled": false, "port": 3426, "running": false, "hasKey": true, "baseUrl": "http://127.0.0.1:3426/v1", "models": nil}
	f.catalog = map[string]any{"codexUsers": []any{}, "builtinPlugins": []any{}, "bundledPlugins": []any{}}
	return f
}

func (f *fakeAI) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	switch method {
	case "capture.ai.configuration.get":
		return f.configuration, nil
	case "capture.ai.configuration.save":
		f.saves = append(f.saves, p)
		configuration := p["configuration"].(map[string]any)
		configuration["revision"] = configuration["revision"].(float64) + 1
		// The Host protects new keys and reports them saved.
		if updates, ok := p["keyUpdates"].(map[string]any); ok {
			for _, provider := range configuration["providers"].([]any) {
				pr := provider.(map[string]any)
				for slot := range updates[pr["id"].(string)].(map[string]any) {
					for _, k := range pr["keys"].([]any) {
						if k.(map[string]any)["id"] == slot {
							k.(map[string]any)["hasKey"] = true
						}
					}
				}
			}
		}
		f.configuration = configuration
		return configuration, nil
	case "capture.ai.accounts.catalog":
		return map[string]any{"agents": []any{map[string]any{"id": "codex", "name": "Codex", "icon": "openai"}},
			"presets": []any{map[string]any{"id": "deepseek", "name": "DeepSeek", "chat": "https://api.deepseek.com/v1", "models": []any{"deepseek-chat"}}}}, nil
	case "capture.ai.accounts.list":
		return map[string]any{"accounts": f.accounts, "methods": []any{map[string]any{"type": "oauth", "label": "Browser"}}}, nil
	case "capture.ai.accounts.signin.start":
		f.signInIndex = 0
		if len(f.signIn) == 0 {
			// A sign-in the test does not follow: it waits for the vendor.
			f.signIn = []map[string]any{{"id": "job", "status": map[string]any{"agent": p["agent"], "url": "https://auth.example/", "state": "canceled"}}}
		}
		return f.signIn[0], nil
	case "capture.ai.accounts.signin.get":
		if f.signInIndex < len(f.signIn)-1 {
			f.signInIndex++
		}
		if f.signIn[f.signInIndex]["status"].(map[string]any)["state"] == "done" {
			f.accounts = []map[string]any{{"agent": "codex", "user": "me@example.com", "plan": "Plus"}}
		}
		return f.signIn[f.signInIndex], nil
	case "capture.ai.accounts.signin.open", "capture.ai.accounts.signin.cancel":
		return f.signIn[f.signInIndex], nil
	case "capture.ai.keys.reveal":
		return map[string]string{"key": f.keys[p["provider"].(string)+"/"+p["id"].(string)]}, nil
	case "capture.ai.models.fetch":
		f.fetched = append(f.fetched, p)
		return map[string]any{"models": []any{map[string]any{"id": "gpt-a", "name": "GPT A"}, map[string]any{"id": "gpt-c", "free": true}}}, nil
	case "capture.ai.gateway.get":
		return f.gateway, nil
	case "capture.ai.gateway.save":
		f.gatewaySaves = append(f.gatewaySaves, p)
		settings := p["settings"].(map[string]any)
		f.gateway["enabled"], f.gateway["port"], f.gateway["models"] = settings["enabled"], settings["port"], settings["models"]
		f.gateway["running"] = settings["enabled"]
		return f.gateway, nil
	case "capture.ai.magpie.catalog":
		return f.catalog, nil
	case "capture.ai.routing.extensions.open":
		return map[string]any{"url": "http://127.0.0.1:9/session/key/plugins.html", "pages": []any{}, "panels": []any{}}, nil
	case "capture.ai.routing.extensions.close", "capture.ai.routing.close", "capture.ai.routing.plugins.close", "capture.ai.magpie.privacy.close":
		return nil, nil
	case "capture.ai.routing.open", "capture.ai.routing.plugins.open":
		return map[string]string{"url": "http://127.0.0.1:9/session/key/"}, nil
	case "capture.ai.magpie.privacy.open", "capture.ai.magpie.privacy.state":
		return map[string]any{"on": false, "supported": true, "installed": false, "size": 1.6e9}, nil
	}
	f.t.Errorf("unexpected %s %s", method, params)
	return nil, nil
}

func (f *fakeAI) lastSave() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saves) == 0 {
		return nil
	}
	return f.saves[len(f.saves)-1]
}

func setup(t *testing.T) (*fakeAI, *testhost.Harness, *page, *ui.Tester) {
	i18n.SetLocale(i18n.Chinese)
	f := newFakeAI(t)
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1100, 1600)
	settle(h, tt)
	t.Cleanup(p.Close)
	return f, h, p, tt
}

// settle also waits for the reads a frame starts: a tab or provider
// shown for the first time reads its state as it draws.
func settle(h *testhost.Harness, tt *ui.Tester) {
	for range 3 {
		h.Settle(tt)
	}
}

// shot saves what the tester shows when WINONE_SHOTS names a folder, as
// visual evidence; tests pass without it.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		return
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestSavingKeepsFieldsThePageDoesNotEdit(t *testing.T) {
	f, h, p, tt := setup(t)
	shot(t, tt, "ai-custom")
	if !tt.HasText("Relay") || !tt.HasText("1 个供应商 · 1 个模型") {
		t.Fatalf("texts %q", tt.Texts())
	}
	p.change("one", func(pr *provider) { pr.BaseURL = "https://other.example/v1" })
	h.Settle(tt)
	if h.Env.Rail.Save.Disabled {
		t.Fatal("an edit does not enable Save")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	saved := f.lastSave()
	if saved == nil {
		t.Fatal("not saved")
	}
	c := saved["configuration"].(map[string]any)
	provider := c["providers"].([]any)[0].(map[string]any)
	if provider["baseUrl"] != "https://other.example/v1" || provider["futureProviderField"] != "kept" || c["prompt"] != "Translate." ||
		c["futureSetting"] == nil || c["magpieSettings"].(map[string]any)["futureMagpie"] != 7.0 {
		t.Fatalf("saved %v", c)
	}
	if _, ok := saved["keyUpdates"]; ok {
		t.Fatal("a save without new keys sends keyUpdates")
	}
	if p.dirty() {
		t.Fatal("still dirty after saving")
	}
}

func TestNewKeysTravelApartAndSavedKeysAreRevealedOnRequest(t *testing.T) {
	f, h, p, tt := setup(t)
	e := p.editor
	if e == nil {
		t.Fatal("no editor")
	}
	// Reveal the saved key: read from the Host, never part of the draft.
	if err := tt.Click(i18n.T("ai.showKey", "name", i18n.T("ai.key"))); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if row := e.keyRows["primary"]; row == nil || !row.visible || row.text != "sk-saved-123" {
		t.Fatalf("revealed %+v", e.keyRows["primary"])
	}
	if p.dirty() {
		t.Fatal("revealing a key changed the draft")
	}
	// Typing a new key is an edit that saves as a key update.
	e.keyRows["primary"].text = "sk-new-456"
	tt.Frame()
	h.Settle(tt)
	if p.keys["one"]["primary"] != "sk-new-456" || !p.dirty() {
		t.Fatalf("key edits %v", p.keys)
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	saved := f.lastSave()
	updates, _ := saved["keyUpdates"].(map[string]any)
	if updates == nil || updates["one"].(map[string]any)["primary"] != "sk-new-456" {
		t.Fatalf("key updates %v", saved["keyUpdates"])
	}
	if len(p.keys) != 0 || e.keyRows["primary"] != nil && e.keyRows["primary"].visible {
		t.Fatal("a save keeps new keys or revealed ones")
	}
}

func TestCurlExampleFillsTheDraftWithoutSaving(t *testing.T) {
	f, h, p, tt := setup(t)
	pr := p.draft.Value.find("one")
	if !p.importCurl("curl https://api.deepseek.com/v1/chat/completions -H \"Authorization: Bearer sk-real-key\" -d '{\"model\":\"deepseek-chat\"}'", pr, "") {
		t.Fatal("not read as cURL")
	}
	h.Settle(tt)
	pr = p.draft.Value.find("one")
	if pr.BaseURL != "https://api.deepseek.com/v1" || pr.Protocol != "chat" || p.keys["one"]["primary"] != "sk-real-key" {
		t.Fatalf("imported %+v keys %v", pr, p.keys)
	}
	// A saved provider's enabled models are not changed by an example.
	if len(pr.Models) != 1 || pr.Models[0] != "gpt-a" {
		t.Fatalf("models %v", pr.Models)
	}
	if !tt.HasText(i18n.T("ai.curlImported")) {
		t.Fatalf("no notice %q", tt.Texts())
	}
	if len(f.saves) != 0 {
		t.Fatal("an import saved")
	}
}

func TestDiscoveryReplacesTheCatalogAndClearsTasksOfRemovedModels(t *testing.T) {
	f, h, p, tt := setup(t)
	if err := tt.Click(i18n.T("ai.fetchModels")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if len(f.fetched) != 1 || f.fetched[0]["provider"] != "one" || f.fetched[0]["connection"] != nil {
		t.Fatalf("fetch %v", f.fetched)
	}
	pr := p.draft.Value.find("one")
	if strings.Join(value(pr.AvailableModels), ",") != "gpt-a,gpt-c" || strings.Join(pr.Models, ",") != "gpt-a" || pr.ModelNames["gpt-a"] != "GPT A" || !pr.ModelFree["gpt-c"] {
		t.Fatalf("catalog %+v", pr)
	}
	// gpt-b left the catalog: the OCR task that used it is cleared.
	if p.draft.Value.Tasks["ocr"].Provider != "" || p.draft.Value.Tasks["default"].Model != "gpt-a" {
		t.Fatalf("tasks %v", p.draft.Value.Tasks)
	}
	shot(t, tt, "ai-models")
}

func TestAnUnsavedConnectionIsSentWithDiscovery(t *testing.T) {
	f, h, p, tt := setup(t)
	p.change("one", func(pr *provider) { pr.BaseURL = "https://draft.example/v1" })
	h.Settle(tt)
	if err := tt.Click(i18n.T("ai.fetchModels")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	connection, _ := f.fetched[0]["connection"].(map[string]any)
	if connection == nil || connection["baseUrl"] != "https://draft.example/v1" {
		t.Fatalf("fetch %v", f.fetched[0])
	}
}

func TestAccountSignInRefreshesTheAccounts(t *testing.T) {
	f, h, p, tt := setup(t)
	f.signIn = []map[string]any{
		{"id": "job", "status": map[string]any{"agent": "codex", "url": "https://auth.example/login", "state": "waiting", "opened": true}},
		{"id": "job", "status": map[string]any{"agent": "codex", "url": "https://auth.example/login", "state": "done"}},
	}
	p.switchTab(tabAccounts)
	settle(h, tt)
	p.add(&agent{ID: "codex", Name: "Codex", Icon: "openai"})
	settle(h, tt)
	if !tt.HasText(i18n.T("ai.noAccount")) {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click(i18n.T("ai.signIn")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("me@example.com") || !tt.HasText(i18n.T("ai.signedIn")) {
		t.Fatalf("after sign-in %q", tt.Texts())
	}
	shot(t, tt, "ai-accounts")
}

func TestGatewayValidatesItsPortAndSavesApart(t *testing.T) {
	f, h, p, tt := setup(t)
	p.switchTab(tabGateway)
	settle(h, tt)
	g := p.gateway
	g.port = "80"
	h.Settle(tt)
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(f.gatewaySaves) != 0 || !tt.HasText(i18n.T("gateway.portError")) {
		t.Fatalf("port 80 saved: %v", f.gatewaySaves)
	}
	g.port, g.enabled = "3500", true
	h.Settle(tt)
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(f.gatewaySaves) != 1 || f.gatewaySaves[0]["settings"].(map[string]any)["port"] != 3500.0 || len(f.saves) != 0 {
		t.Fatalf("gateway saves %v, configuration saves %d", f.gatewaySaves, len(f.saves))
	}
	shot(t, tt, "ai-gateway")
}

func TestTabsAskTheGuard(t *testing.T) {
	_, h, p, tt := setup(t)
	requested := 0
	h.Env.RequestChange = func(apply func()) { requested++ }
	if err := tt.Click(i18n.T("gateway.title")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if requested != 1 || p.tab != tabCustom {
		t.Fatalf("requested %d, tab %d", requested, p.tab)
	}
}

func TestRemovingAProviderClearsItsTasks(t *testing.T) {
	_, h, p, tt := setup(t)
	if err := tt.Click(i18n.T("ai.removeService")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if p.draft.Value.find("one") != nil || p.draft.Value.Tasks["default"].Provider != "" {
		t.Fatalf("after removal %+v", p.draft.Value)
	}
	// The overview keeps the removed provider, unavailable, until saved.
	if !tt.HasText("Relay") {
		t.Fatalf("overview %q", tt.Texts())
	}
}

func TestAdvancedEditsAreDraftsOfTheConfiguration(t *testing.T) {
	f, h, p, tt := setup(t)
	p.switchTab(tabAdvanced)
	settle(h, tt)
	shot(t, tt, "ai-advanced")
	if err := tt.Click(i18n.T("ai.quotaLeft")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !p.draft.Value.magpie().QuotaLeft || !p.dirty() {
		t.Fatal("the choice is not in the draft")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	settings := f.lastSave()["configuration"].(map[string]any)["magpieSettings"].(map[string]any)
	if settings["quotaLeft"] != true || settings["futureMagpie"] != 7.0 {
		t.Fatalf("saved %v", settings)
	}
}

func TestModelSettingsKeepTheRoutingID(t *testing.T) {
	_, h, p, tt := setup(t)
	p.editor.models.editing = "gpt-a"
	h.Settle(tt)
	shot(t, tt, "ai-model-settings")
	if err := tt.Click("200K"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	pr := p.draft.Value.find("one")
	if value(pr.ModelMetadata["gpt-a"].Value.ContextWindow) != 200000 || pr.Models[0] != "gpt-a" {
		t.Fatalf("metadata %+v", pr.ModelMetadata)
	}
}

// The routing page's one action saves routing groups alone: the page's
// other drafts stay drafts, and the draft keeps them with the new
// revision.
func TestRoutingEnablesGroupsWithoutSavingOtherDrafts(t *testing.T) {
	f, h, p, tt := setup(t)
	p.change("one", func(pr *provider) { pr.Name = "Draft name" })
	p.routing.receive(web.Message{"type": "winone-routing-native", "id": "1", "method": "enable-groups"})
	settle(h, tt)
	saved := f.lastSave()["configuration"].(map[string]any)
	groups := saved["magpieSettings"].(map[string]any)["builtinPlugins"].(map[string]any)["groups"].(map[string]any)
	if groups["enabled"] != true || saved["providers"].([]any)[0].(map[string]any)["name"] != "Relay" {
		t.Fatalf("saved %v", saved)
	}
	if p.draft.Value.find("one").Name != "Draft name" || p.draft.Value.Revision != 5 || !p.dirty() {
		t.Fatalf("draft %+v", p.draft.Value)
	}
	if !*p.draft.Value.magpie().BuiltinPlugins["groups"].Value.Enabled {
		t.Fatal("the draft lacks the saved choice")
	}
}

// The plugin manager opens a plugin's account type on the accounts tab,
// and an explicit sign-in waits there for the connection's methods.
func TestPluginManagerOpensItsAccountType(t *testing.T) {
	_, h, p, tt := setup(t)
	p.plugins.receive(web.Message{"type": "winone-plugin-native", "id": "2", "method": "provider", "provider": "acme", "name": "Acme", "signIn": true})
	settle(h, tt)
	pr := p.current()
	if p.tab != tabAccounts || pr == nil || pr.Agent != "acme" || pr.AgentBackend != "plugin" {
		t.Fatalf("tab %d provider %+v", p.tab, pr)
	}
	f := p.extensions.signIn
	if f != nil {
		t.Fatalf("the explicit sign-in was not continued: %+v", f)
	}
	// Opening it again reuses the connection.
	p.plugins.receive(web.Message{"type": "winone-plugin-native", "id": "3", "method": "provider", "provider": "acme", "name": "Acme"})
	count := 0
	for _, d := range p.draft.Value.Providers {
		if d.Value.Agent == "acme" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d connections", count)
	}
}

// Privacy records turn masking back on for kinds: their warn or block
// actions leave the draft.
func TestMaskingRecordsStageTheirKinds(t *testing.T) {
	_, h, p, tt := setup(t)
	p.draft.Value.settings().RedactActions = map[string]string{"EMAIL": "warn", "PHONE": "block"}
	p.switchTab(tabAdvanced)
	settle(h, tt)
	p.advanced.receiveRecords(web.Message{"type": "winone-masking-actions", "kinds": []any{"EMAIL"}})
	if actions := p.draft.Value.magpie().RedactActions; actions["EMAIL"] != "" || actions["PHONE"] != "block" || !p.advanced.focusPrivacy {
		t.Fatalf("actions %v", actions)
	}
	// Kinds not named as Magpie names them are refused.
	p.advanced.receiveRecords(web.Message{"type": "winone-masking-actions", "kinds": []any{"phone"}})
	if p.draft.Value.magpie().RedactActions["PHONE"] != "block" {
		t.Fatal("an invalid kind changed the draft")
	}
}

package automations

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

type fakeScheduler struct {
	channelsReady bool
	mu            sync.Mutex
	rows          []Schedule
	runs          []Run
	saved         map[string]any
	failSave      bool
	deletes       []string
	creates       int
}

func (f *fakeScheduler) handle(method string, raw json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var input map[string]any
	_ = json.Unmarshal(raw, &input)
	switch method {
	case "chat.automations.list":
		return f.rows, nil
	case "chat.automations.runtime":
		return map[string]any{"slotsInUse": 1, "maxConcurrentRuns": 2, "timezone": "Asia/Shanghai", "scriptCapabilities": []string{"github.read", "sessions.dispatch"}}, nil
	case "chat.automations.project.pendingImports":
		return []any{}, nil
	case "chat.automations.events":
		return map[string]any{"cursor": 0, "events": []any{}}, nil
	case "chat.automations.runs":
		return f.runs, nil
	case "chat.automations.create", "chat.automations.update":
		if f.failSave {
			return nil, fmt.Errorf("SAVE_FAILED")
		}
		key := "input"
		if method == "chat.automations.update" {
			key = "patch"
		} else {
			f.creates++
		}
		f.saved = input[key].(map[string]any)
		return Schedule{ID: "created", Definition: Definition{Name: "Saved"}, Status: "active"}, nil
	case "chat.catalog":
		return map[string]any{"conversations": []any{map[string]any{"id": "owner", "title": "Owner chat"}}}, nil
	case "capture.ai.configuration.get":
		return map[string]any{"providers": []any{}}, nil
	case "chat.automations.channels":
		status := "disconnected"
		if f.channelsReady {
			status = "connected"
		}
		return map[string]any{"feishu": map[string]string{"status": status}, "wecomGroup": map[string]bool{"configured": f.channelsReady, "enabled": f.channelsReady}}, nil
	case "chat.automations.capabilityStatus":
		return map[string]any{"statuses": []any{map[string]string{"capability": "sessions.dispatch", "state": "ok"}}}, nil
	case "chat.automations.inflightCount":
		return 1, nil
	case "chat.automations.delete", "chat.automations.pause", "chat.automations.run":
		return map[string]any{"ok": true}, nil
	case "chat.conversation.delete", "chat.conversation.update":
		f.deletes = append(f.deletes, input["id"].(string))
		return nil, nil
	case "chat.automations.templates":
		return map[string]any{"templates": []any{map[string]any{"id": "tpl", "name": "Digest", "source": "builtin", "prompt": "Digest {{topic}}", "parameters": []any{map[string]any{"key": "topic", "label": "Topic", "type": "string", "required": true}}}}}, nil
	case "chat.automations.template.apply":
		return map[string]any{"name": "Digest", "prompt": "Digest Go"}, nil
	default:
		return nil, fmt.Errorf("unexpected method %s", method)
	}
}
func setup(t *testing.T, width int) (*fakeScheduler, *testhost.Harness, *Page, *ui.Tester) {
	i18n.SetLocale(i18n.Chinese)
	f := &fakeScheduler{rows: []Schedule{{ID: "s1", Definition: Definition{Name: "Daily report", Prompt: "Summarize", Kind: "cron", AgentKind: "pi", CronExpr: "0 9 * * *", Timezone: "Asia/Shanghai", TargetSessionID: "owner"}, Status: "active"}}, runs: []Run{{ID: "r1", SessionID: "owner", Status: "success", ResultText: "**Finished**"}, {ID: "r2", SessionID: "generated", Status: "failed", ErrorMsg: "Try again"}}}
	h := testhost.New(t, f.handle)
	p := New(h.Env, Options{})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, width, 760)
	for range 3 {
		h.Settle(tt)
	}
	t.Cleanup(p.Close)
	return f, h, p, tt
}
func TestPluginScheduleDraftOpensTheRealFormWithoutCreatingOrReplacingWork(t *testing.T) {
	f, h, p, tt := setup(t, 1050)
	raw := json.RawMessage(`{"requestId":"draft-one","name":"Plugin reminder","prompt":"Review this reminder","intervalMs":7200000}`)
	if !p.OpenPluginDraft(raw) { t.Fatal("draft was not accepted") }
	for range 3 { h.Settle(tt) }
	if p.draft == nil || p.draft.Value.Name != "Plugin reminder" || p.draft.Value.Prompt != "Review this reminder" || p.draft.timing != "interval" || p.draft.interval != "120" || f.creates != 0 {
		t.Fatalf("wrong draft or premature create: %+v creates=%d", p.draft, f.creates)
	}
	if p.OpenPluginDraft(json.RawMessage(`{"requestId":"draft-two","name":"Replacement","prompt":"Do not overwrite"}`)) { t.Fatal("replaced the user's open form") }
	if p.draft.Value.Name != "Plugin reminder" { t.Fatal("draft was overwritten") }
	p.Discard()
	if p.OpenPluginDraft(raw) { t.Fatal("replayed the same delivered draft") }
	if f.creates != 0 { t.Fatal("opening or dismissing a draft created a schedule") }
}

func TestNativeListHistoryAndCreateUseTheRealHostBridge(t *testing.T) {
	f, h, p, tt := setup(t, 1050)
	if !tt.HasText("Daily report") || !tt.HasText("Finished") || !tt.HasText(tr("create")) {
		t.Fatalf("native content missing: %v", tt.Texts())
	}
	p.openEditor(nil)
	for range 3 {
		h.Settle(tt)
	}
	p.draft.Value.Name = "New check"
	p.draft.Value.Prompt = "Check the report"
	p.save()
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.creates != 1 || f.saved["name"] != "New check" || f.saved["agentKind"] != "pi" {
		t.Fatalf("wrong create receipt: %#v", f.saved)
	}
	if _, ok := f.saved["permissionMode"]; ok {
		t.Fatal("native form must not override the Host's permission default")
	}
	if p.draft != nil {
		t.Fatal("accepted save left an unsaved draft")
	}
}
func TestFailedSaveRetainsDraftAndCloseAsksToDiscard(t *testing.T) {
	f, h, p, tt := setup(t, 900)
	p.openEditor(&p.items[0])
	for range 3 {
		h.Settle(tt)
	}
	p.draft.Value.Name = "Keep my edit"
	f.mu.Lock()
	f.failSave = true
	f.mu.Unlock()
	p.save()
	for range 3 {
		h.Settle(tt)
	}
	if p.draft == nil || p.draft.Value.Name != "Keep my edit" || !p.Dirty() {
		t.Fatal("failed save lost edits")
	}
	dirty, _ := h.Env.Activity.State()
	if !dirty {
		t.Fatal("shell is not protecting the automation draft")
	}
	if p.draft.err == "" {
		t.Fatal("save failure was hidden")
	}
}
func TestDeletionNeverMutatesHandBoundConversation(t *testing.T) {
	f, h, p, tt := setup(t, 900)
	p.deleteSchedule(p.items[0])
	for range 3 {
		h.Settle(tt)
	}
	p.confirmation.disposition = "delete"
	p.confirm()
	for range 3 {
		h.Settle(tt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.deletes) != 1 || f.deletes[0] != "generated" {
		t.Fatalf("wrong conversation deleted: %v", f.deletes)
	}
}
func TestConnectorRefreshKeepsSavedChoicesAndConfigurationUsesTheUnsavedGuard(t *testing.T) {
	f, h, p, _ := setup(t, 900)
	p.openEditor(&p.items[0])
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.notificationsFields(c, p.draft) }, 560, 700)
	for range 3 {
		h.Settle(tt)
	}
	if !p.supportLoaded {
		t.Fatalf("connector status unavailable: %s", p.supportError)
	}
	_ = tt.Click(tr("feishuNotify"))
	if p.draft.Value.Notify.Feishu {
		t.Fatal("an unconfigured channel was enabled")
	}
	p.draft.Value.Notify.Feishu = true
	p.RefreshSupport()
	for range 3 {
		h.Settle(tt)
	}
	if !p.draft.Value.Notify.Feishu {
		t.Fatal("an unavailable connector silently reset its saved selection")
	}
	f.mu.Lock()
	f.channelsReady = true
	f.mu.Unlock()
	p.RefreshSupport()
	for range 3 {
		h.Settle(tt)
	}
	if p.support.Feishu.Status != "connected" {
		t.Fatal("saved connector change did not refresh")
	}
	opened := false
	var pending func()
	p.options.ConfigureConnections = func() { opened = true }
	h.Env.RequestChange = func(action func()) { pending = action }
	p.configureConnections()
	if opened || pending == nil || p.draft == nil {
		t.Fatal("configuration navigation bypassed the draft decision")
	}
	pending()
	if !opened || p.draft != nil {
		t.Fatal("confirmed configuration navigation did not complete")
	}
}

func TestNativeAutomationShots(t *testing.T) {
	_, h, p, _ := setup(t, 1000)
	for _, language := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(language)
		h.Shots(t, "automations-"+language, p.View, 800, nil)
		p.openEditor(&p.items[0])
		h.Shots(t, "automation-form-"+language, p.View, 900, nil)
		p.Discard()
	}
	i18n.SetLocale(i18n.Chinese)
}

func TestTemplatesRequireParametersAndNeverOverwriteAnOpenDraft(t *testing.T) {
	_, h, p, tt := setup(t, 900)
	p.showTemplates()
	for range 3 {
		h.Settle(tt)
	}
	template := p.templates[0]
	p.applyTemplate(template)
	if p.templateErr == "" {
		t.Fatal("missing template parameter accepted")
	}
	p.templateParams["topic"] = "Go"
	p.applyTemplate(template)
	for range 3 {
		h.Settle(tt)
	}
	if p.draft == nil || p.draft.Value.Prompt != "Digest Go" {
		t.Fatal("template output never reached the native draft")
	}
	p.draft.Value.Prompt = "Keep this"
	p.applyTemplate(template)
	if p.draft.Value.Prompt != "Keep this" {
		t.Fatal("template replaced unsaved instructions")
	}
}

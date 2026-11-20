package aiservices

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

func TestConfiguredProviderLeadsWithModelsAndKeepsConnectionDraft(t *testing.T) {
	_, h, p, tt := setup(t)
	models, ok := tt.Find(i18n.T("ai.models"))
	if !ok || models.Y > 360 {
		t.Fatalf("daily models are below the first working area: %+v", models)
	}
	if tt.HasText(i18n.T("ai.address")) {
		t.Fatal("a configured provider starts with its infrequent connection form expanded")
	}
	if err := tt.Click(i18n.T("aiExperience.connection")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	tt.Advance(300 * time.Millisecond)
	input, hasInput := tt.AccessByRole(ui.RoleTextField, i18n.T("ai.address"))
	if !hasInput || input.Offscreen {
		t.Fatalf("expanded connection input is unavailable: %+v", input)
	}
	tt.ClickAt(input.Bounds.X+input.Bounds.W/2, input.Bounds.Y+input.Bounds.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("https://changed.example/v1")
	settle(h, tt)
	if p.current().BaseURL != "https://changed.example/v1" {
		t.Fatalf("connection input did not accept replacement: %q", p.current().BaseURL)
	}
	for _, label := range []string{i18n.T("aiExperience.settings"), i18n.T("aiOptions.billing"), i18n.T("aiExperience.back")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	if p.current().BaseURL != "https://changed.example/v1" || !p.dirty() {
		t.Fatalf("switching settings lost the connection draft: address=%q dirty=%t", p.current().BaseURL, p.dirty())
	}
}

func TestNewPresetShowsConnectionBeforeModels(t *testing.T) {
	_, h, p, tt := setup(t)
	p.addPreset(p.catalog.presets[0])
	settle(h, tt)
	address, hasAddress := tt.Find(i18n.T("ai.address"))
	models, hasModels := tt.Find(i18n.T("ai.models"))
	if !hasAddress || !hasModels || address.Y >= models.Y {
		t.Fatalf("new preset did not lead with the connection: address=%+v models=%+v", address, models)
	}
}

func TestClearingConnectionAddressKeepsTypingFocus(t *testing.T) {
	_, h, p, _ := setup(t)
	p.editor.connectionOpen = true
	tt := ui.NewTester(p.View, 1100, 2400)
	settle(h, tt)
	field, ok := tt.AccessByRole(ui.RoleTextField, i18n.T("ai.address"))
	if !ok {
		t.Fatal("missing address input")
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	settle(h, tt)
	tt.Type("https://replacement.example/v1")
	settle(h, tt)
	if p.current().BaseURL != "https://replacement.example/v1" {
		t.Fatalf("clearing the address interrupted typing: %q", p.current().BaseURL)
	}
}

func TestProviderSettingsKeepMaintenanceOutOfDailyModels(t *testing.T) {
	f, h, p, tt := setup(t)
	p.current().AuthMode, p.current().Agent = "account", "codex"
	p.saved.Value.find("one").AuthMode, p.saved.Value.find("one").Agent = "account", "codex"
	f.accounts = []map[string]any{{"agent": "codex", "user": "daily@example.invalid"}}
	p.switchTab(tabAccounts)
	settle(h, tt)
	if tt.HasText(i18n.T("aiSync.warmNow")) || tt.HasText("Codex CLI") {
		t.Fatal("maintenance parameters still interrupt daily model management")
	}
	for _, label := range []string{i18n.T("aiExperience.settings"), i18n.T("aiOptions.accounts")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	if !tt.HasText(i18n.T("aiSync.warmNow")) || !tt.HasText("Codex CLI") {
		t.Fatal("account maintenance is not available in provider settings")
	}
}

func TestAccountWithoutSessionShowsSignInBeforeModels(t *testing.T) {
	_, h, p, tt := setup(t)
	p.current().AuthMode, p.current().Agent = "account", "codex"
	p.saved.Value.find("one").AuthMode, p.saved.Value.find("one").Agent = "account", "codex"
	p.switchTab(tabAccounts)
	settle(h, tt)
	login, loginOK := tt.Find(i18n.T("ai.signIn"))
	models, modelsOK := tt.Find(i18n.T("ai.models"))
	if !loginOK || !modelsOK || login.Y >= models.Y {
		t.Fatalf("missing account session is hidden below models: login=%+v models=%+v", login, models)
	}
}

// Render production views with actual controls, realistic names and both
// locales. No pixel replica is allowed to stand in for the shipped UI.
func TestAIExperienceLayouts(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{560, 1000} {
				for scene := 0; scene < 38; scene++ {
					f, h, p, _ := setup(t)
					i18n.SetLocale(locale)
					pr, e := p.current(), p.editor
					view := p.View
					switch {
					case scene == 0:
						p.switchTab(tabHome)
					case scene == 1:
						p.draft.Value.Providers = nil
						p.draft.Value.Tasks = nil
						p.saved = ptr(clone(*p.draft))
						p.switchTab(tabHome)
					case scene == 3:
						e.connectionOpen = true
					case scene == 4 || scene == 5:
						p.choosingPreset = true
						p.catalog.presets = append(p.catalog.presets, servicePreset{ID: "long", Name: "An independent provider with a much longer name", endpoints: endpoints{Chat: "https://example.invalid/v1"}, Note: "Choose a service, then enter your own API key. This description must wrap without moving the name or the action."})
						if scene == 5 {
							p.presetSearch = "no-matching-provider"
						}
					case scene == 6:
						pr.Name = "An independent provider with a much longer name"
						p.bulkProvidersOpen = true
					case scene == 7:
						p.choosingAgent = true
					case scene >= 8 && scene < 14:
						e.optionsOpen = true
						e.preferenceTab = scene - 8
					case scene >= 14 && scene < 18:
						e.models.editing = pr.Models[0]
						e.models.settingsTab = scene - 14
					case scene >= 18 && scene < 21:
						p.switchTab(tabGateway)
						p.gateway.connectOpen = scene == 19
						p.gateway.networkOpen = scene == 20
					case scene == 21 || scene == 22:
						view = func(c *ui.Context) { e.avatarView(c, pr, false) }
						e.avatar.mode = "builtin"
						if scene == 22 {
							e.avatar.mode = "url"
						}
					case scene >= 23 && scene < 26:
						pr.AuthMode, pr.Agent = "account", "codex"
						p.saved.Value.find(pr.ID).AuthMode, p.saved.Value.find(pr.ID).Agent = "account", "codex"
						if scene != 23 {
							f.accounts = []map[string]any{{"agent": "codex", "user": "daily@example.invalid", "plan": "Plus"}}
						}
						p.switchTab(tabAccounts)
						p.editor = newEditor(p, pr, pr.ID+":"+pr.Agent+":"+pr.AgentBackend)
						p.editor.optionsOpen, p.editor.preferenceTab = scene == 25, 1
					case scene >= 26 && scene < 32:
						a := &p.gateway.access
						a.fields = fields{}
						a.catalog = &accessCatalog{Models: []accessChoice{{ID: "one/gpt-a", Name: "GPT A", ProviderName: "Relay"}}}
						if scene < 29 {
							a.creating, a.temporary, a.name, a.ttl = true, scene == 27, "Desktop client", "1h"
							a.browser = scene == 28
							view = func(c *ui.Context) { a.createView(c, p) }
						} else {
							a.begin(p, callerKey{ID: "fixture", Name: "Desktop client", Limit: &callerLimit{Period: "day", Tokens: 200000, Cost: 5}})
							a.tab = scene - 29
							view = func(c *ui.Context) { a.editorView(c, p) }
						}
					case scene == 32:
						a := &p.gateway.access
						a.identityOpen, a.rules, a.fields = true, []gatewayOIDCRule{}, fields{}
						a.rule.Claims = map[string]string{"repository": "team/project"}
						view = func(c *ui.Context) { a.oidcView(c, p) }
					case scene == 33:
						p.catalog.err, p.choosingPreset = "Service list could not be loaded. Retry when your connection is available.", true
					case scene == 34:
						p.choosingPreset, p.catalog.busy = true, true
					case scene == 35:
						e.models.err = "The provider did not return a model list. Check the address and key, then try again."
					case scene == 36:
						p.gateway.calls.loaded = true
						p.gateway.calls.routes = []gatewayCall{{ID: 1, Time: "10:24", Agent: "Relay", Model: "GPT A", Done: true}, {ID: 2, Time: "10:26", Agent: "Relay", Model: "GPT B", Error: "Connection timed out", Status: 504}}
						view = func(c *ui.Context) { p.gateway.recentCallsView(c, p) }
					case scene == 37:
						p.gateway.runtimeURL = "http://127.0.0.1:1/session/layout/"
						g := &p.gateway.history
						g.source, g.state = p.gateway.runtimeURL, &gatewayHistoryState{}
						view = func(c *ui.Context) { kit.Disclosure(c, accessT("historyTab"), "", new(true), func() { g.View(c, p) }) }
					}
					tt := ui.NewTester(func(c *ui.Context) {
						c.SetTheme(kit.Theme(c.Preferences(), dark))
						ui.Scroll(c).Fill().Padding(20).Children(func() { view(c) })
					}, width, 900)
					settle(h, tt)
					// Inspect the expanded gateway content itself, below the
					// resizable model list, rather than recapturing its first screen.
					if scene == 19 || scene == 20 {
						label := upstreamT("Connect a client")
						if scene == 20 {
							label = accessT("networkTab")
						}
						if b, ok := tt.Find(label); ok {
							tt.Scroll(float32(width-10), 450, 0, b.Y-40)
							tt.Advance(300 * time.Millisecond)
							settle(h, tt)
						}
					}
					for _, text := range tt.Texts() {
						if b, ok := tt.Find(text); ok && b.Y >= 0 && b.Y < 900 && (b.X < -.5 || b.X+b.W > float32(width)+.5) {
							t.Errorf("scene=%d %s dark=%t width=%d: %q clipped at %+v", scene, locale, dark, width, text, b)
						}
					}
					shot(t, tt, fmt.Sprintf("experience-%02d-%s-%t-%d", scene, locale, dark, width))
					p.Close()
				}
			}
		}
	}
}

func TestAIExperienceSupportsLargeText(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	_, h, p, _ := setup(t)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, scale := range []float32{1.5, 2} {
			for scene := 0; scene < 3; scene++ {
				p.choosingPreset, p.bulkProvidersOpen = scene == 1, scene == 2
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), false))
					ui.Scroll(c).Fill().Padding(20).Children(func() { p.View(c) })
				}, 880, 1000)
				tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
				settle(h, tt)
				for _, text := range tt.Texts() {
					if b, ok := tt.Find(text); ok && b.Y >= 0 && b.Y < 1000 && (b.X < -.5 || b.X+b.W > 880.5) {
						t.Errorf("%s scale=%.1f scene=%d %q overflows: %+v", locale, scale, scene, text, b)
					}
				}
				shot(t, tt, fmt.Sprintf("experience-large-%s-%.1f-%d", locale, scale, scene))
			}
		}
	}
}

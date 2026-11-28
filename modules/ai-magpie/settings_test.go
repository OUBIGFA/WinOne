package main

import (
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewRuntimeSettingsReachOriginalBudgetAndPrivacyPolicy(t *testing.T) {
	old := settings.WinOneSettings.Load()
	t.Cleanup(func() { settings.WinOneSettings.Store(old) })
	if err := applyRuntimeSettings(request{MagpieSettings: json.RawMessage(`{"timeZone":"Asia/Shanghai","hideRequestText":true,"claudeWaysOut":["US"],"claudeRegionHold":true,"codexAutoReset":["fixture@example.invalid"],"codexResetDaysLeast":1,"codexResetDaysMost":7}`)}); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	if s.Location().String() != "Asia/Shanghai" || !s.HideRequestText || !slices.Equal(s.ClaudeWaysOut, []string{"US"}) {
		t.Fatal("runtime policy lost")
	}
	lo, hi := s.ResetDays()
	if lo != 24*time.Hour || hi != 7*24*time.Hour {
		t.Fatal("reset lifetime changed")
	}
	if !s.ClaudeRegionHold || !provider.CodexAutoReset("fixture@example.invalid") {
		t.Fatal("account policy did not reach execution")
	}
}

// Every account in WarmAll gets the original per-account wait. The adapter
// must not impose a three-minute cap over its queued batches.
func TestWorkerWarmAllKeepsCallerLifetimeAndMoveKeepsLongWait(t *testing.T) {
	parent, stop := context.WithTimeout(t.Context(), 4*time.Minute)
	defer stop()
	warm, cancel := workerRequestContext(parent, request{Operation: "accounts.warm-all"})
	defer cancel()
	want, _ := parent.Deadline()
	got, _ := warm.Deadline()
	if !got.Equal(want) {
		t.Fatalf("warm-all shortened caller lifetime: %s, want %s", got, want)
	}
	move, done := workerRequestContext(parent, request{Operation: "accounts.move-agy"})
	defer done()
	if end, _ := move.Deadline(); time.Until(end) < 2*time.Minute {
		t.Fatal("migration still uses the short ordinary worker wait")
	}
	stop()
	if warm.Err() != context.Canceled {
		t.Fatal("caller cancellation was lost")
	}
}

// The settings bridge must feed the upstream signer and model listing, rather
// than retaining choices only in the native form's private document.
func TestRuntimeCodexClientAndBareModelListingFollowUpstream(t *testing.T) {
	old := settings.WinOneSettings.Load()
	oldProviders := provider.WinOneProviders
	t.Cleanup(func() {
		settings.WinOneSettings.Store(old)
		provider.WinOneProviders = oldProviders
		provider.Changed()
	})
	for _, mode := range []struct {
		client   string
		on, only bool
		ids      []string
	}{{"desktop", false, false, []string{"fixture/only-model"}}, {"", true, false, []string{"fixture/only-model", "only-model"}}, {"desktop", true, true, []string{"only-model"}}} {
		raw, _ := json.Marshal(map[string]any{"codexClient": mode.client, "bareModelIDs": mode.on, "bareModelIDsOnly": mode.only})
		if err := applyRuntimeSettings(request{MagpieSettings: raw}); err != nil {
			t.Fatal(err)
		}
		if provider.CodexClient() != mode.client {
			t.Fatalf("runtime Codex client is %q, want %q", provider.CodexClient(), mode.client)
		}
		provider.WinOneProviders = []provider.Provider{{ID: "fixture", Name: "Fixture", Chat: "https://vendor.invalid/v1", Key: "fixture-key", Models: []string{"only-model"}}}
		provider.Changed()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+gateway.Token)
		gateway.New().Handler().ServeHTTP(w, r)
		var reply struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &reply) != nil {
			t.Fatalf("offline model listing failed: %d %s", w.Code, w.Body.String())
		}
		var ids []string
		for _, model := range reply.Data {
			if strings.Contains(model.ID, "only-model") {
				ids = append(ids, model.ID)
			}
		}
		if !slices.Equal(ids, mode.ids) {
			t.Fatalf("model ID mode on=%v only=%v listed %v, want %v", mode.on, mode.only, ids, mode.ids)
		}
	}
}

// Generation uses upstream's header/stall guards and retains the owner's
// cancellation. Account operations still receive their existing bounded wait.
func TestWorkerModelContextKeepsCallerDeadlineAndCancellation(t *testing.T) {
	for _, operation := range []string{"", "generate"} {
		ctx, cancel := workerRequestContext(context.Background(), request{Operation: operation})
		if deadline, bounded := ctx.Deadline(); bounded {
			t.Errorf("model operation %q added an implicit deadline: %v", operation, deadline)
		}
		cancel()
		if ctx.Err() != context.Canceled {
			t.Fatal("model worker lost its owner cancellation")
		}
		deadline := time.Now().Add(time.Hour)
		parent, stop := context.WithDeadline(context.Background(), deadline)
		ctx, cancel = workerRequestContext(parent, request{Operation: operation})
		if actual, bounded := ctx.Deadline(); !bounded || !actual.Equal(deadline) {
			t.Errorf("model worker changed an explicit caller deadline: %v", actual)
		}
		cancel()
		stop()
	}
}

// Old full-version switches are data only and cannot revive removed services.
func TestRuntimeSettingsIgnoreRemovedServices(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	in := request{MagpieSettings: json.RawMessage(`{"claudePrintMode":true,"redact":true,"redactModel":true,"builtinPreset":"all","builtinPlugins":{"routing":{"enabled":true}},"requestArchiveLocal":true,"otel":{"enabled":true},"noStats":false}`)}
	if err := applyRuntimeSettings(in); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	if !s.ClaudePrintMode || !s.NoWSLAgents || !s.NoStats || !s.NoUsageStats || !s.RedactModelOff || s.Redact || s.RedactModel || s.RequestArchiveLocal || s.OTel.Enabled {
		t.Fatal("removed runtime settings became active")
	}
}

func TestWorkerImageDeadlinePreservesBothUpstreamPhases(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	for _, minutes := range []int{0, 1, 30, 60} {
		settings.WinOneUse(settings.Settings{ImageTimeout: minutes})
		want := 2*settings.Load().ImageWait() + time.Minute
		for _, operation := range []string{"gateway"} {
			for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
				if got := workerRequestWait(request{Operation: operation, GatewayPath: path}); got != want {
					t.Fatalf("%s %s: %v, want %v", operation, path, got, want)
				}
			}
			if workerRequestWait(request{Operation: operation, GatewayPath: "/v1/chat/completions"}) != 10*time.Minute {
				t.Fatal("image timeout changed a normal protocol deadline")
			}
		}
	}
	if workerRequestWait(request{Operation: "generate"}) != 0 {
		t.Fatal("image timeout added a whole-turn deadline to a model request")
	}
}

func TestAdditionalToolsAndNativeSearchModelMetadata(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	defer func() { provider.WinOneProviders = nil; provider.WinOneResetSearchModels() }()
	p, err := configuredProvider(request{Protocol: "responses", BaseURL: "https://fixture.invalid/v1", AdditionalTools: true})
	if err != nil || !p.AdditionalTools {
		t.Fatal("provider additional_tools not preserved", err)
	}
	if err := installAuxiliaryProviders(request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "fixture", Name: "Fixture", Chat: "http://127.0.0.1:1/v1", Models: []string{"search-model", "plain-model"}}, Metadata: map[string]modelMetadata{"search-model": {Searches: true}}}}}); err != nil {
		t.Fatal(err)
	}
	models := provider.WinOneProviders[0].Available()
	found := false
	for _, m := range models {
		if m.ID == "search-model" {
			found = m.WebSearch == "native"
		}
		if m.ID == "plain-model" && m.WebSearch == "native" {
			t.Fatal("native search leaked to another model")
		}
	}
	if !found {
		t.Fatal("auxiliary model lost native search capability")
	}
}

// Validate the upstream lookup, not merely the JSON field direction.
func TestModelAliasKeepsWireAndCompaction(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	settings.WinOneUse(settings.Settings{})
	p := provider.Provider{ID: "fixture", Models: []string{"vendor-model"}}
	if err := installModelPreferences(p.ID, "vendor-model", modelMetadata{Alias: "my-model", ContextWindow: 128000, CompactionPercent: 72.5}); err != nil {
		t.Fatal(err)
	}
	if got := p.OwnModelID("my-model"); got != "vendor-model" {
		t.Fatalf("wire model: %s", got)
	}
	if got := settings.Load().ModelCompacts["fixture/my-model"]; got != 92800 {
		t.Fatalf("compact tokens: %d", got)
	}
	if err := installModelPreferences(p.ID, "vendor-model", modelMetadata{}); err != nil {
		t.Fatal(err)
	}
	if got := p.OwnModelID("my-model"); got != "my-model" {
		t.Fatalf("cleared alias still active: %s", got)
	}
	if _, exists := settings.Load().ModelCompacts["fixture/my-model"]; exists {
		t.Fatal("cleared alias retained its model overrides")
	}
}

func TestAuxiliaryModelNamesAndAliasContext(t *testing.T) {
	old, providers := settings.WinOneSettings.Load(), provider.WinOneProviders
	defer func() { settings.WinOneSettings.Store(old); provider.WinOneProviders = providers }()
	settings.WinOneUse(settings.Settings{})
	provider.WinOneProviders = nil
	metadata := map[string]modelMetadata{"original": {Alias: "friendly", ContextWindow: 64000}}
	if err := installAuxiliaryProviders(request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "fixture", Models: []string{"original"}}, Metadata: metadata, ModelNames: map[string]string{"original": "My model"}}}}); err != nil {
		t.Fatal(err)
	}
	if got := provider.WinOneProviders[0].ContextOf("friendly"); got != 64000 {
		t.Fatalf("alias context = %d", got)
	}
	if got := settings.Load().ModelNames["fixture/friendly"]; got != "My model" {
		t.Fatalf("alias name = %q", got)
	}
	if err := installModelNames("fixture", nil, metadata); err != nil {
		t.Fatal(err)
	}
	if len(settings.Load().ModelNames) != 0 {
		t.Fatal("cleared names remained active")
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/masking"
	"github.com/yetone/magpie/internal/settings"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRuntimeRedactionRestoresResponseAndUsesWireName(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer func() {
		settings.WinOneSettings.Store(old)
		provider.WinOneSearchAPIs = nil
		provider.WinOneProviders = nil
	}()
	var receivedModel, receivedText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"id": "gpt-4.1"}}})
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		receivedModel = body.Model
		for _, m := range body.Messages {
			if value, ok := m.Content.(string); ok && strings.Contains(value, "project") {
				receivedText = value
			}
		}
		if receivedText == "" {
			for _, m := range body.Messages {
				if value, ok := m.Content.(string); ok {
					receivedText = value
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "test", "object": "chat.completion", "model": body.Model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": receivedText}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	in := request{Protocol: "chat", BaseURL: server.URL + "/v1", Model: "gpt-4.1", Text: "private-project", ModelMetadata: modelMetadata{UpstreamName: "wire-model"}, MagpieSettings: json.RawMessage(`{"redact":true,"redactWords":["private-project"]}`)}
	encoded, _ := json.Marshal(in)
	out, err := run(context.Background(), bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if receivedModel != "wire-model" {
		t.Fatalf("wire name not used: %q", receivedModel)
	}
	if strings.Contains(receivedText, "private-project") {
		t.Fatal("unmasked text reached vendor")
	}
	if out != "private-project" {
		t.Fatalf("response was not restored: %q", out)
	}
}

func TestTraySelectionPreservesWholeAccountWindows(t *testing.T) {
	q := provider.SubscriptionQuota{Provider: "codex", User: "a", Windows: []provider.QuotaWindow{{Name: "week", Span: 7 * 24 * time.Hour}, {Name: "model", Model: "one"}, {Name: "extra", Aside: true}, {Name: "five", Span: 5 * time.Hour}}}
	cards := trayCards([]provider.SubscriptionQuota{q}, []string{"saved-provider|a"}, "saved-provider", time.Now())
	if len(cards) != 1 || len(cards[0].Windows) != 2 || cards[0].Windows[0].Name != "five" {
		t.Fatalf("wrong tray windows: %+v", cards)
	}
}

// These assertions exercise the snapshots consumed by the upstream gateway,
// not a duplicate WinOne redaction or search implementation.
func TestRuntimeSettingsReachUpstream(t *testing.T) {
	in := request{MagpieSettings: json.RawMessage(`{"redact":true,"redactShape":true,"redactPersonal":true,"redactWords":["private-project"],"vision":"off","searcher":"off","searchFirst":"api","noLoopGuard":true,"searchAPIs":[{"vendor":"tavily","key":"test-key"}]}`)}
	if err := applyRuntimeSettings(in); err != nil {
		t.Fatal(err)
	}
	defer func() { settings.WinOneSettings.Store(nil); provider.WinOneSearchAPIs = nil }()
	s := settings.Load()
	if !s.Redact || !s.RedactShape || !s.RedactPersonal || s.Vision != "off" || s.Searcher != "off" || s.SearchFirst != "api" || !s.NoLoopGuard {
		t.Fatalf("settings not applied: %+v", s)
	}
	if !s.NoWSLAgents {
		t.Fatal("embedded runtime must never discover real WSL agent profiles")
	}
	apis := provider.SearchAPIs()
	if len(apis) != 1 || apis[0].Key != "test-key" {
		t.Fatal("search API was not installed")
	}
}

func TestWorkerImageDeadlinePreservesBothUpstreamPhases(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	for _, minutes := range []int{0, 1, 30, 60} {
		settings.WinOneUse(settings.Settings{ImageTimeout: minutes})
		want := 2*settings.Load().ImageWait() + time.Minute
		for _, operation := range []string{"gateway", "magpie.gateway"} {
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
	if workerRequestWait(request{Operation: "generate"}) != 90*time.Second {
		t.Fatal("image timeout changed the short result task deadline")
	}
}

func TestRuntimeRejectsInvalidRedactionRule(t *testing.T) {
	if err := applyRuntimeSettings(request{MagpieSettings: json.RawMessage(`{"redactRules":[{"kind":"PRIVATE","regex":"["}]}`)}); err == nil {
		t.Fatal("invalid regex accepted")
	}
}

func TestPrivacyActionsReachVendorAndPreview(t *testing.T) {
 old:=settings.WinOneSettings.Load();defer settings.WinOneSettings.Store(old)
 defer func(){provider.WinOneProviders=nil}()
 for _,test:=range []struct{name,prefs string;sent,clear bool}{
  {"mask",`{"redact":true,"redactWords":["private-project"]}`,true,false},
  {"ignore-case",`{"redact":true,"redactWords":["PRIVATE-PROJECT"],"redactWordsAnyCase":true}`,true,false},
  {"warn",`{"redact":true,"redactWords":["private-project"],"redactActions":{"TERM":"warn"}}`,true,true},
  {"block",`{"redact":true,"redactWords":["private-project"],"redactActions":{"TERM":"block"}}`,false,false},
  {"allow",`{"redact":true,"redactWords":["private-project"],"redactActions":{"TERM":"block"},"redactAllow":["private-project"]}`,true,true},
 } {t.Run(test.name,func(t *testing.T){
  var received string;called:=false
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){called=true;var body map[string]any;json.NewDecoder(r.Body).Decode(&body);b,_:=json.Marshal(body);received=string(b);w.Header().Set("Content-Type","application/json");w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))}));defer server.Close()
  in:=request{Protocol:"chat",BaseURL:server.URL+"/v1",Model:"fixture",Text:"private-project",MagpieSettings:json.RawMessage(test.prefs)}
  data,_:=json.Marshal(in);_,err:=run(t.Context(),bytes.NewReader(data))
  if test.sent&&err!=nil {t.Fatal(err)}
  if called!=test.sent||called&&strings.Contains(received,"private-project")!=test.clear {t.Fatalf("wrong privacy action: sent=%v body=%q err=%v",called,received,err)}
  text,hits,_:=masking.Preview("private-project")
  if test.name=="allow" {if len(hits)!=0||text!="private-project" {t.Fatalf("allowlist ignored: %q %v",text,hits)}} else if len(hits)==0 {t.Fatal("preview omitted findings")}
 })}
}

func TestAdditionalToolsAndNativeSearchModelMetadata(t *testing.T) {
 old:=settings.WinOneSettings.Load();defer settings.WinOneSettings.Store(old)
 defer func(){provider.WinOneProviders=nil;provider.WinOneSearchModels=nil}()
 p,err:=configuredProvider(request{Protocol:"responses",BaseURL:"https://fixture.invalid/v1",AdditionalTools:true})
 if err!=nil||!p.AdditionalTools {t.Fatal("provider additional_tools not preserved",err)}
 if err:=installAuxiliaryProviders(request{AuxiliaryProviders:[]auxiliaryProvider{{Provider:provider.Provider{ID:"fixture",Name:"Fixture",Chat:"http://127.0.0.1:1/v1",Models:[]string{"search-model","plain-model"}},Metadata:map[string]modelMetadata{"search-model":{Searches:true}}}}});err!=nil {t.Fatal(err)}
 models:=provider.WinOneProviders[0].Available();found:=false
 for _,m:=range models {if m.ID=="search-model" {found=m.WebSearch=="native"};if m.ID=="plain-model"&&m.WebSearch=="native" {t.Fatal("native search leaked to another model")}}
 if !found {t.Fatal("auxiliary model lost native search capability")}
}

func TestClassifyTimeoutReachesUpstream(t *testing.T) {
 old:=settings.WinOneSettings.Load();defer settings.WinOneSettings.Store(old)
 for _,n:=range []int{0,1,23,60} {
  raw,_:=json.Marshal(map[string]int{"classifyTimeout":n})
  if err:=applyRuntimeSettings(request{MagpieSettings:raw});err!=nil {t.Fatal(err)}
  want:=n;if want==0 {want=8}
  if got:=settings.Load().ClassifyWait();got!=time.Duration(want)*time.Second {t.Fatalf("timeout=%v want=%d",got,want)}
 }
}

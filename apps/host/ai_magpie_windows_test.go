//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
	"winone/internal/settings"
)

func TestModelConnectivityRejectsInvalidDraftsWithoutChangingSettings(t *testing.T) {
	engine := newAIService(context.Background(), t.TempDir(), "WinOne.Host.exe")
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	engine.store = store
	initial := store.Configuration()
	initial.Providers = []ai.Provider{{ID: "saved", Name: "Saved", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}}}
	if _, err = store.SaveConfiguration(initial, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{
		`{"provider":"saved","models":["model"],"as":"unsupported"}`,
		`{"provider":"saved","models":["model"],"draft":{"agent":"codex"},"connection":{"protocol":"chat","baseUrl":"https://example.invalid/v1"}}`,
		`{"provider":"saved","models":["model"],"keyUpdates":{"primary":"draft-key"}}`,
		`{"provider":"saved","models":["model"],"metadata":{"model":{"maxTokens":-1}}}`,
	} {
		response := engine.handle(host.Request{ID: "test", Method: "models.test", Params: json.RawMessage(params)})
		if response.Error == nil || strings.Contains(response.Error.Message, "不支持的 AI 操作") {
			t.Fatalf("invalid test request was not validated: %+v", response)
		}
	}
	after, err := os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("model test changed the saved settings")
	}
}

func TestMagpieSearchKeyIsRevealedOnlyByExplicitMethod(t *testing.T) {
	directory := t.TempDir()
	config, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(config)
	engine := newAIService(context.Background(), directory, filepath.Join(directory, "WinOne.Host.exe"))
	bindAIService(service, engine)
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	input := store.Configuration()
	input.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true, SearchAPIs: []ai.SearchAPI{{Vendor: "tavily", Key: "private-search-fixture"}}}
	if _, err = store.SaveConfiguration(input, nil); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"ai.configuration.get", "capture.ai.configuration.get"} {
		response := service.Handle(host.Request{Version: 1, ID: "public", Method: method})
		if response.Error != nil || strings.Contains(string(response.Result), "private-search-fixture") {
			t.Fatalf("public preferences leaked a search key: %v", response.Error)
		}
	}
	for _, method := range []string{"ai.magpie.search-key", "capture.ai.magpie.search-key"} {
		response := service.Handle(host.Request{Version: 1, ID: "reveal", Method: method, Params: json.RawMessage(`{"vendor":"tavily"}`)})
		var value struct {
			Key string `json:"key"`
		}
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		if err = json.Unmarshal(response.Result, &value); err != nil || value.Key != "private-search-fixture" {
			t.Fatal("explicit reveal did not return the stored key")
		}
	}
	for _, params := range []string{`{"vendor":"unknown"}`, `{"vendor":"tavily","extra":true}`, `{}`} {
		response := engine.handle(host.Request{ID: "invalid", Method: "magpie.search-key", Params: json.RawMessage(params)})
		if response.Error == nil {
			t.Fatalf("invalid search key request accepted: %s", params)
		}
	}
}

func TestMagpieCatalogFailureIsExplicitAndDoesNotRewriteSettings(t *testing.T) {
	directory := t.TempDir()
	engine := newAIService(context.Background(), directory, filepath.Join(directory, "WinOne.Host.exe"))
	if err := os.WriteFile(engine.storePath, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	response := engine.handle(host.Request{ID: "catalog", Method: "magpie.catalog", Params: json.RawMessage(`{}`)})
	if response.Error == nil || !strings.Contains(response.Error.Message, "设置") {
		t.Fatalf("broken settings were masked: %+v", response)
	}
	data, err := os.ReadFile(engine.storePath)
	if err != nil || string(data) != "invalid" {
		t.Fatal("catalog rewrote invalid settings")
	}
}

func TestPackagedUsageAliasDraftAndSaveTransaction(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	directory := t.TempDir()
	engine := newAIService(ctx, directory, filepath.Join(directory, "WinOne.Host.exe"))
	engine.translator.Executable = executable
	store, err := engine.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	configuration, err = store.SaveConfiguration(configuration, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	validate := engine.handle(host.Request{ID: "draft", Method: "magpie.usage-aliases", Params: json.RawMessage(`{"aliases":{},"edit":{"from":" DEEPSEEK_V4.1_FLASH ","to":"deepseek-flash"}}`)})
	if validate.Error != nil {
		t.Fatal(validate.Error)
	}
	var draft struct {
		Aliases map[string]string `json:"usageAliases"`
	}
	if err := json.Unmarshal(validate.Result, &draft); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("validation changed saved settings")
	}
	configuration.MagpieSettings.UsageAliases = draft.Aliases
	save := func(value ai.Configuration) host.Response {
		raw, _ := json.Marshal(map[string]any{"configuration": value})
		return engine.handle(host.Request{ID: "save", Method: "configuration.save", Params: raw})
	}
	response := save(configuration)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if err := json.Unmarshal(response.Result, &configuration); err != nil {
		t.Fatal(err)
	}
	reopened, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Configuration().MagpieSettings.UsageAliases["DEEPSEEK_V4.1_FLASH"] != "deepseek-flash" {
		t.Fatal("saved aliases did not survive reopening")
	}
	runtime, err := engine.magpieTranslator()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := runtime.Usage(ctx, ai.UsageQuery{Period: "all", Models: []string{"deepseek-v4.1-flash"}})
	if err != nil {
		t.Fatal(err)
	}
	var usage struct {
		Aliases map[string]string `json:"modelAliases"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.Aliases["deepseek-v4.1-flash"] != "deepseek-flash" {
		t.Fatalf("saved settings did not reach worker: %s", raw)
	}
	before, err = os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	configuration.MagpieSettings.UsageAliases["deepseek-flash"] = "deepseek-v4.1-flash"
	if rejected := save(configuration); rejected.Error == nil {
		t.Fatal("cyclic alias was saved")
	}
	after, err = os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected alias changed saved settings")
	}
}

func TestPackagedModelDiscoveryUsesSavedGlobalProxy(t *testing.T){
 executable:=os.Getenv("WINONE_AI_TEST_EXE");if executable==""{t.Skip("set WINONE_AI_TEST_EXE")}
 called:=false
 proxy:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){called=true;if r.URL.Host!="model-list.invalid"||r.URL.Path!="/v1/models"{t.Errorf("unexpected destination %s",r.URL)};w.Header().Set("Content-Type","application/json");w.Write([]byte(`{"data":[{"id":"proxy-model"}]}`))}));defer proxy.Close()
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 directory:=t.TempDir();engine:=newAIService(ctx,directory,filepath.Join(directory,"WinOne.Host.exe"));engine.translator.Executable=executable
 store,err:=engine.magpieStore();if err!=nil{t.Fatal(err)}
 config:=store.Configuration();config.Providers=[]ai.Provider{{ID:"fixture",Name:"Fixture",AuthMode:"api-key",Protocol:"chat",BaseURL:"http://model-list.invalid/v1",Models:[]string{}}};config.Tasks=map[string]ai.Selection{}
	config.MagpieSettings=&ai.MagpieSettings{Proxy:proxy.URL,NoStats:true,NoUsageStats:true}
	key:="model-proxy-fixture-key"
	if _,err:=store.SaveConfiguration(config,map[string]*string{"fixture":&key});err!=nil{t.Fatal(err)}
 result:=engine.handle(host.Request{ID:"models",Method:"models.fetch",Params:json.RawMessage(`{"provider":"fixture"}`)})
 if result.Error!=nil{t.Fatal(result.Error)}
 if !called||!bytes.Contains(result.Result,[]byte("proxy-model")){t.Fatalf("proxy model discovery failed: %s",result.Result)}
}

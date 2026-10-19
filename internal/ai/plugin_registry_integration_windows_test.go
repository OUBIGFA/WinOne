//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The router's provider bridge starts a private transport worker. A user hook
// belongs to the outer request, even though both workers load the same store.
func TestInstalledPluginRunsOnceThroughRoutingProviderBridge(t *testing.T) {
	executable := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated Magpie bundle")
	}
	observed := make(chan string, 4)
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) > 0 {
			observed <- body.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"Fixture answer\"},\"finish_reason\":\"stop\"}]}")
	}))
	defer vendor.Close()
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	router, err := runtime.StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"model"}}, Key: "fixture-key"}})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	fixture := t.TempDir()
	for file, body := range map[string]string{"package.json": "{\"name\":\"winone-hook-fixture\",\"version\":\"1.0.0\",\"magpie\":{\"hooks\":\"request.js\"}}", "request.js": "export function onRequest(body){return {...body,messages:body.messages.map((m,i)=>i?m:{...m,content:m.content+'|hook'})}}"} {
		if err := os.WriteFile(filepath.Join(fixture, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	invoke := func(path string, body any) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, router.URL+path, bytes.NewReader(data))
		request.Header.Set("Authorization", "Bearer "+router.Key)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("%s returned %d: %s", path, response.StatusCode, raw)
		}
		return raw
	}
	invoke("/api/plugins/add", map[string]string{"spec": fixture})
	invoke("/v1/chat/completions", map[string]any{"model": "fixture/model", "messages": []map[string]string{{"role": "user", "content": "Hello"}}})
	if got := <-observed; got != "Hello|hook" {
		t.Fatalf("hook must run once, upstream received %q", got)
	}
	// A short-lived direct Chat transport still loads and executes the plugin.
	var output bytes.Buffer
	response := httptest.NewRecorder()
	_, err = runtime.ForProvider("direct").ChatRequest(ctx, Preferences{ProviderID: "fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "model", MagpieSettings: &MagpieSettings{NoStats: true, NoUsageStats: true}}, "fixture-key", "/v1/chat/completions", json.RawMessage("{\"model\":\"model\",\"messages\":[{\"role\":\"user\",\"content\":\"Direct\"}]}"), http.Header{}, response)
	if err != nil {
		t.Fatal(err)
	}
	output.Write(response.Body.Bytes())
	if got := <-observed; got != "Direct|hook" {
		t.Fatalf("direct request bypassed hook: %q", got)
	}
	if !strings.Contains(output.String(), "Fixture answer") {
		t.Fatal("direct response lost")
	}
}

func TestInstalledThirdPartyAccountUsesPrivateProviderProfile(t *testing.T) {
	executable := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated Magpie bundle")
	}
	root := t.TempDir()
	runtime := Translator{Executable: executable, DataDirectory: root, AccountBackend: "plugin"}
	fixture := filepath.Join(root, "third-party-provider.mjs")
	source := "export default async()=>({config(c){c.provider??={};c.provider.thirdparty={name:'Third-party fixture',models:{m1:{name:'Model One'}}}},auth:{provider:'thirdparty',methods:[{type:'api',label:'Fixture key'}]}})"
	if err := os.WriteFile(fixture, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(root, "third-party", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	writeRegistry := func(off bool) {
		t.Helper()
		data, _ := json.Marshal(map[string]any{"plugins": []map[string]any{{"spec": fixture, "off": off}}})
		if err := os.WriteFile(registry, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeRegistry(false)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, id := range []string{"first", "second"} {
		worker := runtime.ForProvider(id)
		auth := filepath.Join(worker.DataDirectory, "config", "magpie", "plugin-auth.json")
		if err := os.MkdirAll(filepath.Dir(auth), 0700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"thirdparty": map[string]string{"type": "api", "key": "fixture-" + id, "email": id + "@example.invalid"}})
		if err := os.WriteFile(auth, data, 0600); err != nil {
			t.Fatal(err)
		}
		result, err := worker.Accounts(ctx, "accounts.list", "thirdparty")
		if err != nil {
			t.Fatal(err)
		}
		var listing struct {
			Accounts []struct{ User string }
			Methods  []json.RawMessage
		}
		if err := json.Unmarshal(result, &listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.Methods) != 1 || len(listing.Accounts) != 1 || listing.Accounts[0].User != id+"@example.invalid" {
			t.Fatalf("third-party account isolation failed: %s", result)
		}
	}
	writeRegistry(true)
	if _, err := runtime.ForProvider("first").Accounts(ctx, "accounts.list", "thirdparty"); err == nil {
		t.Fatal("disabled account plugin was used")
	}
}

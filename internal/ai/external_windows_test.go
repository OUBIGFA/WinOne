//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalEnvironmentKeepsClientLocationsAndPrivateData(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "client")
	private := filepath.Join(root, "private")
	env := externalEnvironment(private, []string{"USERPROFILE=" + client, "APPDATA=" + filepath.Join(client, "roaming"), "XDG_CONFIG_HOME=" + filepath.Join(client, "xdg"), "CODEX_HOME=" + filepath.Join(client, "codex"), "OPENAI_API_KEY=must-not-enter"})
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[strings.ToUpper(key)] = value
	}
	if values["USERPROFILE"] != client || values["CODEX_HOME"] != filepath.Join(client, "codex") || values["XDG_CONFIG_HOME"] != filepath.Join(client, "xdg") {
		t.Fatal("external client locations lost")
	}
	if values["WINONE_AI_EXTERNAL_DATA"] != filepath.Join(private, "config", "magpie") {
		t.Fatal("private Magpie store missing")
	}
	if values["OPENAI_API_KEY"] != "" {
		t.Fatal("credential inherited")
	}
}

func TestExternalEnabledReadsOnlyItsPrivateMarker(t *testing.T) {
	root := t.TempDir()
	translator := Translator{DataDirectory: root}
	on, err := translator.ExternalEnabled()
	if err != nil || on {
		t.Fatalf("unconfigured external service: %v %v", on, err)
	}
	path := filepath.Join(root, "config", "magpie", "external-service.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true,"port":45678,"key":"fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	on, err = translator.ExternalEnabled()
	if err != nil || !on {
		t.Fatalf("saved explicit enable lost: %v %v", on, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := translator.ExternalEnabled(); err == nil {
		t.Fatal("damaged private marker was silently accepted")
	}
}

func TestExternalExporterStopUsesHostOnlyChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/winone/external/settings" || r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("Origin") != "" {
			t.Error("incorrect external Host channel")
		}
		var in struct {
			On             bool
			MagpieSettings *MagpieSettings
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.On || in.MagpieSettings != nil {
			t.Error("stop requires no settings or credentials")
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	process := &RoutingProcess{URL: server.URL, Key: "fixture-key"}
	if err := process.ExternalExport(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
}

func TestExternalEnvironmentUsesSharedPluginRegistryRoot(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "external-clients")
	env := externalEnvironment(private, nil, root)
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[strings.ToUpper(key)] = value
	}
	if values["WINONE_AI_PLUGINS_HOME"] != filepath.Join(root, "third-party") {
		t.Fatalf("plugin registry escaped shared root: %q", values["WINONE_AI_PLUGINS_HOME"])
	}
	if values["WINONE_AI_EXTERNAL_DATA"] != filepath.Join(private, "config", "magpie") {
		t.Fatalf("private data moved: %q", values["WINONE_AI_EXTERNAL_DATA"])
	}
}

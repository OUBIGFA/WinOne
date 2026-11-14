//go:build windows

package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type providerPaste struct {
	URL, API, Base, URLHole, Key, Placeholder, Model, Name string
	Bases                                                  [][2]string
}

func providerPasteReader(t *testing.T) func(string) *providerPaste {
	t.Helper()
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	runtime := Translator{Executable: executable, DataDirectory: t.TempDir()}
	return func(text string) *providerPaste {
		t.Helper()
		raw, err := runtime.ImportProviderSetup(t.Context(), text)
		if err != nil {
			t.Fatal(err)
		}
		var r struct{ Parsed *providerPaste }
		if err = json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		return r.Parsed
	}
}

// The vendor examples the React window's tests read: the same fixtures
// give the same imports.
func TestUpstreamPasteReadsVendorExamples(t *testing.T) {
	read := providerPasteReader(t)
	cases := []struct{ file, api, base, model string }{
		{"openai-chat.sh", "openai", "https://api.openai.com/v1", ""},
		{"openai-responses.sh", "responses", "https://api.openai.com/v1", "gpt-6-astra"},
		{"anthropic.sh", "anthropic", "https://api.anthropic.com", "claude-opus-5-5"},
		{"deepseek.sh", "openai", "https://api.deepseek.com/v1", "deepseek-flash"},
		{"zhipu.sh", "openai", "https://open.bigmodel.cn/api/paas/v4", "glm-5.3"},
		{"moonshot.sh", "openai", "https://api.moonshot.cn/v1", "kimi-k3"},
		{"openrouter.sh", "openai", "https://openrouter.ai/api/v1", "~openai/gpt-sol-latest"},
		{"bailian-cmd.bat", "openai", "https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "qwen3.8-max"},
		{"sap-ai-core.ps1", "", "", "meta--llama3.1-70b-instruct"},
		{"unsloth-studio.ps1", "openai", "http://127.0.0.1:8888/v1", ""},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(filepath.Join("..", "..", "apps", "desktop", "frontend", "tests", "fixtures", "curl", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		lf := strings.ReplaceAll(string(data), "\r\n", "\n")
		for _, text := range []string{lf, strings.ReplaceAll(lf, "\n", "\r\n")} {
			got := read(text)
			if got == nil || got.API != tc.api || got.Base != tc.base || got.Model != tc.model || got.Key != "" {
				t.Errorf("%s: %+v", tc.file, got)
			}
		}
	}
}

func TestUpstreamPasteReadsKeysAndEnvironment(t *testing.T) {
	read := providerPasteReader(t)
	if read("https://example.invalid/v1") != nil {
		t.Fatal("an address is not a cURL command")
	}
	got := read("curl https://example.invalid/v1/messages -H 'x-api-key: fixture-token-123'")
	if got == nil || got.API != "anthropic" || got.Base != "https://example.invalid" || got.Key != "fixture-token-123" {
		t.Fatalf("%+v", got)
	}
	got = read(`curl https://api.example.invalid/v1/chat/completions -H "Authorization: Bearer $OPENAI_API_KEY"`)
	if got == nil || got.Key != "" || got.Placeholder != "$OPENAI_API_KEY" {
		t.Fatalf("placeholder %+v", got)
	}
}

func TestUpstreamPasteKeepsEveryEndpointAndNormalizesCopiedText(t *testing.T) {
	read := providerPasteReader(t)
	got := read("export ANTHROPIC_BASE_URL=\"https://relay.invalid/anthropic\"\nexport OPENAI_BASE_URL=\"https://relay.invalid/v1\"\nexport OPENAI_API_KEY=\"fixture-pasted-key\"")
	if got == nil || len(got.Bases) != 2 || got.Key != "fixture-pasted-key" {
		t.Fatalf("environment setup lost fields: %+v", got)
	}
	got = read("cu\u200brl https://relay.invalid/v1/messages -H ‘x-api-key: fixture-pasted-key’")
	if got == nil || got.API != "anthropic" || got.Key != "fixture-pasted-key" {
		t.Fatalf("copied text differs from upstream: %+v", got)
	}
}

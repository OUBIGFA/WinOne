package aiservices

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The vendor examples the React window's tests read: the same fixtures
// give the same imports.
func TestCurlReadsVendorExamples(t *testing.T) {
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
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "desktop", "frontend", "tests", "fixtures", "curl", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		lf := strings.ReplaceAll(string(data), "\r\n", "\n")
		for _, text := range []string{lf, strings.ReplaceAll(lf, "\n", "\r\n")} {
			got := curlOf(text)
			if got == nil || got.API != tc.api || got.Base != tc.base || got.Model != tc.model || got.Key != "" {
				t.Errorf("%s: %+v", tc.file, got)
			}
		}
	}
}

func TestCurlReadsAKeyAndLeavesOtherPastes(t *testing.T) {
	if curlOf("https://example.invalid/v1") != nil {
		t.Fatal("an address is not a cURL command")
	}
	got := curlOf("curl https://example.invalid/v1/messages -H 'x-api-key: fixture-token-123'")
	if got == nil || got.API != "anthropic" || got.Base != "https://example.invalid" || got.Key != "fixture-token-123" {
		t.Fatalf("%+v", got)
	}
	got = curlOf(`curl https://api.example.invalid/v1/chat/completions -H "Authorization: Bearer $OPENAI_API_KEY"`)
	if got == nil || got.Key != "" || got.Placeholder != "$OPENAI_API_KEY" {
		t.Fatalf("placeholder %+v", got)
	}
}

func TestHeadersFollowMagpie(t *testing.T) {
	got := headersOf([]headerRow{{"X-Team", "first", false}, {"x-team", " last ", false}, {"Empty", "", false},
		{"Version", "2.0", false}, {"X-JSON", "{\n \"mode\": \"on\"\n}", true}})
	want := map[string]string{"x-team": "last", "Version": "2.0", "X-JSON": `{"mode":"on"}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if prettyJSON(`{"a":1}`) != "{\n  \"a\": 1\n}" {
		t.Fatal(prettyJSON(`{"a":1}`))
	}
}

func TestProxyLoginRoundTrips(t *testing.T) {
	value := "socks5://na%40me:p%3Ass@127.0.0.1:1080"
	f := proxyFields(value)
	if f.Address != "socks5://127.0.0.1:1080" || f.Username != "na@me" || f.Password != "p:ss" {
		t.Fatalf("%+v", f)
	}
	if got, ok := proxyAddress(f.Address, f, f.Username, f.Password); !ok || got != value {
		t.Fatal(got)
	}
	// An escape that is not UTF-8 stays until its field is edited.
	f = proxyFields("http://user:%FF@proxy.invalid:8080")
	if got, _ := proxyAddress(f.Address, f, f.Username, f.Password); got != "http://user:%FF@proxy.invalid:8080" {
		t.Fatal(got)
	}
	if got, _ := proxyAddress(f.Address, f, "new@user", "new pass"); got != "http://new%40user:new%20pass@proxy.invalid:8080" {
		t.Fatal(got)
	}
}

func TestRuleKindsNormalizeAsMagpie(t *testing.T) {
	for in, want := range map[string]string{" api-key ": "API_KEY", "my.token": "MY_TOKEN", "__": "CUSTOM", "密钥": "CUSTOM"} {
		if got := ruleKind(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

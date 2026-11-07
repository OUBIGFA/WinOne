package mcpapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

const testViewID = "00000000-0000-4000-8000-000000000001"

func validSession() Session {
	var session Session
	session.ViewID, session.Transport = testViewID, "sandbox"
	session.Bridge.BaseURL = "http://127.0.0.1:43123/" + testViewID
	session.Bridge.Key = strings.Repeat("a", 64)
	return session
}
func TestMcpAppSessionAcceptsOnlyNativePrivateHostTransport(t *testing.T) {
	valid := validSession()
	if err := validateSession(valid); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*Session){
		func(s *Session) { s.ViewID = "../../other" }, func(s *Session) { s.Transport = "html" }, func(s *Session) { s.Bridge.Key = "guest-key" },
		func(s *Session) { s.Bridge.BaseURL = "http://example.com/" + s.ViewID }, func(s *Session) { s.Bridge.BaseURL = "http://127.0.0.1:43123/other" },
		func(s *Session) { s.Bridge.BaseURL += "?token=guest" }, func(s *Session) { s.Bridge.BaseURL = "http://user:pass@127.0.0.1:43123/" + s.ViewID },
	} {
		session := valid
		alter(&session)
		if validateSession(session) == nil {
			t.Fatalf("accepted invalid native session: %+v", session)
		}
	}
}
func TestMcpAppProtocolNeverAdmitsHeaderForgedNativeCaller(t *testing.T) {
	request := httptest.NewRequest("POST", "cindy-mcp-app://"+testViewID+"/bridge", strings.NewReader(`{"op":"poll"}`))
	request.Header.Set("Origin", browserOrigin(testViewID, false))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	writer := httptest.NewRecorder()
	serve(writer, request)
	if writer.Code != http.StatusNotFound {
		t.Fatalf("a plain HTTP caller acquired native protocol: %d", writer.Code)
	}
}
func TestMcpAppOriginSeparatesAppFilesFromTrustedProxy(t *testing.T) {
	tests := []struct {
		host, path, method, origin string
		allowed                    bool
	}{
		{testViewID, "/sandbox", "GET", "", true},
		{testViewID, "/bridge", "POST", browserOrigin(testViewID, false), true},
		{testViewID, "/bridge", "POST", browserOrigin(testViewID, true), false},
		{testViewID, "/bridge", "POST", "null", false}, {testViewID, "/bridge", "GET", browserOrigin(testViewID, false), false},
		{testViewID + "-view", "/bridge", "POST", browserOrigin(testViewID, true), false},
		{testViewID + "-view", "/__cindy/files/" + testViewID, "GET", browserOrigin(testViewID, true), true},
		{testViewID + "-view", "/__cindy/files/" + testViewID, "POST", browserOrigin(testViewID, true), false},
		{testViewID + "-view", "/__cindy/files/" + testViewID, "GET", "https://declared.example", false},
		{testViewID + "-view", "/__cindy/files/../../private", "GET", "", false},
	}
	for _, test := range tests {
		u := &url.URL{Scheme: "cindy-mcp-app", Host: test.host, Path: test.path}
		request := &http.Request{URL: u, Method: test.method, Header: http.Header{}}
		request.Header.Set("Origin", test.origin)
		if _, allowed := allowedRequest(testViewID, request); allowed != test.allowed {
			t.Errorf("%s %s %s admitted=%v", test.method, test.host, test.path, allowed)
		}
	}
	for _, address := range []string{"cindy-mcp-app://" + testViewID + "-view/resource?other=1", "cindy-mcp-app://user@" + testViewID + "/sandbox", "cindy-mcp-app://other/sandbox", "cindy-mcp-app://" + testViewID + ":80/sandbox"} {
		parsed, err := url.Parse(address)
		if err == nil {
			if _, ok := requestView(parsed); ok {
				t.Errorf("invalid source accepted: %s", address)
			}
		}
	}
}
func TestMcpAppSetupUsesNativePasswordAndKeepsEveryAnyOfChoice(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	raw := json.RawMessage(`{"kind":"plugin_setup","requestId":"setup-request","revision":7,"ghost":{"id":"fixture","name":"Fixture"},"steps":[{"id":"manual","groupId":"provider","groupMode":"any_of","title":"Manual token","description":"Native secret only","phase":"pending","action":{"id":"opaque-manual","kind":"inline_form","form":{"fields":[{"id":"value","type":"secret","label":"Token","required":true,"maxLength":4096}]}}},{"id":"oauth","groupId":"provider","groupMode":"any_of","title":"OAuth alternative","phase":"pending","action":{"id":"opaque-oauth","kind":"oauth_connect"}}]}`)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		prompt, err := NewSetupPrompt(raw)
		if err != nil {
			t.Fatal(err)
		}
		var answer map[string]any
		tester := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			ui.Column(c).Fill().Padding(20).Children(func() {
				if value, ok := prompt.View(c); ok {
					answer = value
				}
			})
		}, 560, 700)
		if !tester.HasText("Manual token") || !tester.HasText("OAuth alternative") {
			t.Fatalf("any-of choice hidden: %q", tester.Texts())
		}
		if err := tester.Click("Token"); err != nil {
			t.Fatal(err)
		}
		tester.Type("fixture-native-secret")
		if err := tester.Click(i18n.T("pluginHost.save")); err != nil {
			t.Fatal(err)
		}
		if answer["action"] != "submit" || answer["actionId"] != "opaque-manual" || answer["value"] != "fixture-native-secret" {
			t.Fatalf("native private form did not submit its bound field")
		}
		if len(prompt.values) != 0 {
			t.Fatal("submitted native secrets remained in card state")
		}
	}
}

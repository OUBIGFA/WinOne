package pluginhost

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

const nativeSetupRequest = `{"kind":"plugin_setup","requestId":"setup-native","revision":7,"ghost":{"id":"fixture","name":"Fixture"},"steps":[{"id":"manual","groupId":"provider","groupMode":"any_of","title":"Manual token","phase":"pending","action":{"id":"opaque-manual","kind":"inline_form","form":{"fields":[{"id":"value","type":"secret","label":"Token","required":true,"maxLength":4096}]}}},{"id":"oauth","groupId":"provider","groupMode":"any_of","title":"OAuth alternative","phase":"pending","action":{"id":"opaque-oauth","kind":"oauth_connect"}}]}`

func TestNativePluginSetupSubmitsSecretsOnlyThroughThePrivateUIEndpoint(t *testing.T) {
	var active atomic.Bool
	active.Store(true)
	raw := json.RawMessage(`{"kind":"plugin-setup","ghostName":"Fixture","request":` + nativeSetupRequest + `}`)
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "chat.ghosts.pending":
			prompts := []*prompt{}
			if active.Load() {
				prompts = append(prompts, &prompt{ID: "native-prompt", Request: raw})
			}
			return map[string]any{"cursor": 1, "prompts": prompts}, nil
		case "chat.ghosts.connection-submit":
			var input struct {
				ID     string
				Answer map[string]any
			}
			_ = json.Unmarshal(params, &input)
			if input.ID != "native-prompt" || input.Answer["action"] != "submit" || input.Answer["value"] != "fixture-native-secret" || input.Answer["actionId"] != "opaque-manual" {
				t.Errorf("private UI submission lost its bound action")
			}
			active.Store(false)
			return true, nil
		case "chat.ghosts.answer":
			t.Error("secret used the ordinary answer endpoint")
			return true, nil
		}
		return []any{}, nil
	})
	h := For(harness.Env)
	defer h.Close()
	h.apply(1, nil, []*prompt{{ID: "native-prompt", Request: raw}})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); h.View(c) }, 800, 700)
	harness.Settle(tt)
	if !tt.HasText("Manual token") || !tt.HasText("OAuth alternative") {
		t.Fatalf("native any-of choices were hidden: %q", tt.Texts())
	}
	if err := tt.Click("Token"); err != nil {
		t.Fatal(err)
	}
	tt.Type("fixture-native-secret")
	if err := tt.Click(text("save")); err != nil {
		t.Fatal(err)
	}
	harness.Settle(tt)
	if len(harness.Called("chat.ghosts.connection-submit")) != 1 || len(harness.Called("chat.ghosts.answer")) != 0 || len(h.prompts) != 0 {
		t.Fatal("private submit did not settle its real UI request")
	}
}
func TestNativePluginSetupCancelDoesNotSendASecretOrFakeSuccess(t *testing.T) {
	raw := json.RawMessage(`{"kind":"plugin-setup","ghostName":"Fixture","request":` + nativeSetupRequest + `}`)
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "chat.ghosts.pending" {
			return map[string]any{"cursor": 1, "prompts": []*prompt{{ID: "cancel-prompt", Request: raw}}}, nil
		}
		if method == "chat.ghosts.answer" {
			var input struct{ Answer map[string]any }
			_ = json.Unmarshal(params, &input)
			if input.Answer["action"] != "cancel" || input.Answer["value"] != nil {
				t.Error("cancel disclosed a secret or accepted the connection")
			}
		}
		return true, nil
	})
	h := For(harness.Env)
	defer h.Close()
	h.apply(1, nil, []*prompt{{ID: "cancel-prompt", Request: raw}})
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); h.View(c) }, 800, 700)
	harness.Settle(tt)
	if err := tt.Click("Token"); err != nil {
		t.Fatal(err)
	}
	tt.Type("unsent-private-value")
	if err := tt.Click(text("cancel")); err != nil {
		t.Fatal(err)
	}
	harness.Settle(tt)
	if len(harness.Called("chat.ghosts.answer")) != 1 || len(harness.Called("chat.ghosts.connection-submit")) != 0 {
		t.Fatal("cancel used the wrong channel")
	}
}

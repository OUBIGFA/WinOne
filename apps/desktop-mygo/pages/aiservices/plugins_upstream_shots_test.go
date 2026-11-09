package aiservices

import (
	"os"
	"testing"
	"winone/desktopmygo/i18n"
)

// Production native controls, representative upstream records and no external
// fetches. The snapshots are inspected alongside the real upstream page.
func TestUpstreamPluginScreens(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit snapshots")
	}
	_, h, p, _ := setup(t)
	p.switchTab(tabPlugins)
	x := p.plugins
	x.source = p.extensions.state.URL
	x.marketLoaded = true
	x.githubLoaded = true
	x.state = &managedPlugins{Bun: true, BunVersion: "1.4.2", Plugins: []managedPlugin{{Spec: "@example/provider", OwnName: "Example subscription", Version: "1.0.0", Latest: "1.1.0", Providers: []string{"Example"}, OptionsExample: map[string]any{"region": "global"}}}}
	x.market = []marketPlugin{
		{Package: "@example/provider", Name: "Example subscription", Kind: "provider", Community: true, Version: "1.1.0", Summary: map[string]string{"en": "Claude, GPT and Gemini models from your subscription.", "zh": "使用订阅中的 Claude、GPT 和 Gemini 模型。"}},
		{Package: "@example/middleware", Name: "Request rules", Kind: "middleware", Community: true, Version: "2.0.0", Summary: map[string]string{"en": "Apply your rules to requests and replies.", "zh": "为请求和回复应用自定义规则。"}},
	}
	p.advanced = newAdvancedPanel(p)
	p.advanced.catalog = &magpieCatalog{BuiltinPlugins: []builtinPlugin{{ID: "routing", Name: "Routing", Sub: "Each request as the gateway routes it, the accounts it weighed and what each answered, and your routing groups", On: true}, {ID: "redact", Name: "Privacy masking", Sub: "Secrets, personal data and your own words go to vendors as placeholders, and come back as they were", On: true}}}
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, tab := range []int{0, 1, 2} {
			x.tab = tab
			h.Shots(t, "upstream-plugin-"+locale+"-"+string(rune('0'+tab)), p.View, 1000, nil)
		}
	}
	i18n.SetLocale(i18n.Chinese)
}

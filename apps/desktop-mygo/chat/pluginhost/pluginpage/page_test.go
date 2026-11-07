package pluginpage

import (
	"net/url"
	"strings"
	"testing"
)

func TestPluginPageAcceptsOnlyTheHostIssuedLocalPage(t *testing.T) {
	scope := strings.Repeat("a", 64)
	base := "http://127.0.0.1:42/" + scope + "/"
	for _, address := range []string{base + "__page_settings__", base + "__page_main__", base + "__page_panel__", base + "__page_task__?session=ordinary-task"} {
		if _, err := issuedPageURL(address); err != nil {
			t.Errorf("Host page rejected: %s: %v", address, err)
		}
	}
	for _, address := range []string{
		"https://127.0.0.1:42/" + scope + "/__page_settings__",
		"http://localhost:42/" + scope + "/__page_settings__",
		"http://user@127.0.0.1:42/" + scope + "/__page_settings__",
		"http://127.0.0.1:42/other/__page_settings__",
		base + "arbitrary-preview.html", base + "__boot__", base + "__page_settings__?session=other",
		base + "__page_task__", base + "__page_task__?session=one&session=two", base + "__page_task__?session=one&grant=other",
	} {
		if _, err := issuedPageURL(address); err == nil {
			t.Errorf("Non-plugin document acquired plugin page handling: %s", address)
		}
	}
}

func TestPluginPageInternalNavigationCannotCrossThePrivateScope(t *testing.T) {
	scope := strings.Repeat("a", 64)
	initial, _ := issuedPageURL("http://127.0.0.1:42/" + scope + "/__page_settings__")
	for _, address := range []string{
		initial.String() + "#heading",
		"http://127.0.0.1:42/" + scope + "/ui/details.html",
	} {
		next, _ := url.Parse(address)
		if !samePluginDocument(initial, next) {
			t.Errorf("Own plugin document blocked: %s", address)
		}
	}
	for _, address := range []string{
		"http://127.0.0.1:42/" + strings.Repeat("b", 64) + "/__page_settings__",
		"http://127.0.0.1:43/" + scope + "/__page_settings__",
		"https://example.com/", "file:///C:/private.txt", "javascript:alert(1)",
		"http://user@127.0.0.1:42/" + scope + "/__page_settings__",
	} {
		next, _ := url.Parse(address)
		if samePluginDocument(initial, next) {
			t.Errorf("Cross-scope navigation stayed in the guest: %s", address)
		}
	}
}

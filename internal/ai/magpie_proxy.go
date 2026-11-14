// Generated from yetone/magpie ce2ce86ab43a185688556b5887efab6f1670c58b; MIT.
// Host uses inline proxies; named references are checked against an empty upstream list.
// See third_party/magpie-proxy-source.json.
package ai

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type magpieNamedProxy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// URL is http://, https:// or socks5:// (host:port means http), with
	// no user and password in it.
	URL  string `json:"url"`
	User string `json:"user,omitempty"`
	Pass string `json:"pass,omitempty"`
}

const magpieProxyRefPrefix = "@"

func isMagpieProxyRef(choice string) bool {
	return strings.HasPrefix(strings.TrimSpace(choice), magpieProxyRefPrefix)
}

func magpieProxyID(choice string) string {
	choice = strings.TrimSpace(choice)
	if !strings.HasPrefix(choice, magpieProxyRefPrefix) {
		return ""
	}
	return strings.TrimPrefix(choice, magpieProxyRefPrefix)
}

func validateMagpieProxyIn(p string, ps []magpieNamedProxy) error {
	p = strings.TrimSpace(p)
	if isMagpieProxyRef(p) {
		id := magpieProxyID(p)
		if !slices.ContainsFunc(ps, func(n magpieNamedProxy) bool { return n.ID == id }) {
			return fmt.Errorf("the saved proxy %q isn't there", id)
		}
		return nil
	}
	return validateMagpieProxy(p)
}

func validateMagpieProxy(p string) error {
	p = strings.TrimSpace(p)
	if p == "" || p == "direct" {
		return nil
	}
	if isMagpieProxyRef(p) {
		// a saved proxy: one that is there now
		return validateMagpieProxyIn(p, nil)
	}
	raw := p
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !slices.Contains([]string{"http", "https", "socks5", "socks5h"}, u.Scheme) {
		return fmt.Errorf("proxy must look like http://127.0.0.1:7890 or socks5://127.0.0.1:1080")
	}
	return nil
}

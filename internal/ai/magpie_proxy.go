// Generated from yetone/magpie c39d36644742a10a4730f0ffc5a03d877f185541; MIT.
// Only package and function names change; see third_party/magpie-proxy-source.json.
package ai

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func validateMagpieProxy(p string) error {
	p = strings.TrimSpace(p)
	if p == "" || p == "direct" {
		return nil
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

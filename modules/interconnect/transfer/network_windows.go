//go:build windows

package main

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/sys/windows/registry"
)

// Read only: use the proxy the user already enabled in Windows. Never change
// TUN routing, system DNS, PAC or registry values on behalf of an upload.
func systemTransferProxy() func(*http.Request) (*url.URL, error) {
	direct := func(*http.Request) (*url.URL, error) { return nil, nil }
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return direct
	}
	defer key.Close()
	enabled, _, _ := key.GetIntegerValue("ProxyEnable")
	server, _, _ := key.GetStringValue("ProxyServer")
	if enabled == 0 || server == "" {
		return direct
	}
	override, _, _ := key.GetStringValue("ProxyOverride")
	cfg := httpproxy.Config{NoProxy: strings.ReplaceAll(override, ";", ",")}
	if strings.Contains(server, "=") {
		for _, part := range strings.Split(server, ";") {
			kind, address, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(kind)) {
			case "http":
				cfg.HTTPProxy = strings.TrimSpace(address)
			case "https":
				cfg.HTTPSProxy = strings.TrimSpace(address)
			}
		}
	} else {
		cfg.HTTPProxy = server
		cfg.HTTPSProxy = server
	}
	choose := cfg.ProxyFunc()
	return func(r *http.Request) (*url.URL, error) {
		if strings.Contains(override, "<local>") && !strings.Contains(r.URL.Hostname(), ".") {
			return nil, nil
		}
		return choose(r.URL)
	}
}

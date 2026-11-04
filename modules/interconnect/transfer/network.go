package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// Go's default transport does not read Windows' system proxy. Both cloud
// services now use the same precedence as ordinary croc: explicit SOCKS5,
// explicit HTTP, environment, then the already-enabled system proxy. Keep
// standard CONNECT/TLS/DNS behavior: a proxy's rule-mode routing is its user's
// policy, not something an upload should silently override (or retry POSTs to
// evade). TUN alone needs no special socket or certificate treatment.
func newTransferHTTP(spec specification) (*http.Client, error) {
	proxyFor, err := transferProxy(spec)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFor
	transport.ResponseHeaderTimeout = 45 * time.Second
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("cloud transfer: unexpected redirect")
	}}, nil
}

func transferProxy(spec specification) (func(*http.Request) (*url.URL, error), error) {
	address, scheme := spec.Socks5, "socks5"
	if address == "" {
		address, scheme = spec.HTTPProxy, "http"
	}
	if address != "" {
		if !strings.Contains(address, "://") {
			address = scheme + "://" + address
		}
		u, err := url.Parse(address)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return nil, errors.New("cloud transfer: invalid proxy address")
		}
		return http.ProxyURL(u), nil
	}
	environment := httpproxy.FromEnvironment()
	envProxy := environment.ProxyFunc()
	system := systemTransferProxy()
	return func(r *http.Request) (*url.URL, error) {
		// A configured environment proxy returning nil means NO_PROXY,
		// not permission to fall through to another proxy.
		if r.URL.Scheme == "https" && environment.HTTPSProxy != "" || r.URL.Scheme == "http" && environment.HTTPProxy != "" {
			return envProxy(r.URL)
		}
		return system(r)
	}, nil
}

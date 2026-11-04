//go:build !windows

package main

import (
	"net/http"
	"net/url"
)

// On other systems the existing environment-proxy path is authoritative.
func systemTransferProxy() func(*http.Request) (*url.URL, error) {
	return func(*http.Request) (*url.URL, error) { return nil, nil }
}

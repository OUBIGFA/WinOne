package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/yetone/magpie/internal/provider"
)

// Plugin authorization currently exposes its device code through instructions,
// not a structured field. Keep this boundary adaptation independent of provider
// names; a code pasted back to the app is a different direction of input.
var signInDeviceCode = regexp.MustCompile(`(?i:\b(?:device\s+)?code\b\s*(?:is\s*)?[:：]?\s+)([A-Z0-9]{4,}(?:-[A-Z0-9]+)*|[A-Z0-9]+(?:-[A-Z0-9]+)+)\b`)
var signInChineseDeviceCode = regexp.MustCompile(`(?:设备码|设备代码)\s*(?:是\s*)?[:：]?\s*([A-Z0-9]{4,}(?:-[A-Z0-9]+)*)`)

func normalizeSignInState(state provider.SignInState) provider.SignInState {
	if state.Code != "" || state.State != "waiting" || state.PasteCode || state.PasteCallback || state.PasteKey {
		return state
	}
	for _, pattern := range []*regexp.Regexp{signInDeviceCode, signInChineseDeviceCode} {
		if match := pattern.FindStringSubmatch(state.Instructions); match != nil {
			state.Code = match[1]
			return state
		}
	}
	// RFC 8628 verification links may include the public user_code. Never read
	// OAuth's generic code or the private device_code polling credential.
	if target, err := url.Parse(state.URL); err == nil && target.Scheme == "https" && target.Hostname() != "" && target.User == nil {
		code := strings.TrimSpace(target.Query().Get("user_code"))
		if len(code) <= 256 && !strings.ContainsAny(code, "\r\n\x00") {
			state.Code = code
		}
	}
	return state
}

//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"strings"
)

func supportedLanguage(preferred []string) string {
	for _, locale := range preferred {
		code := strings.ToLower(locale)
		if code == "zh" || strings.HasPrefix(code, "zh-") {
			return "zh-CN"
		}
		if code == "en" || strings.HasPrefix(code, "en-") {
			return "en-US"
		}
	}
	return "en-US"
}

// Windows' preferred UI language list respects per-user language settings;
// English is the packaged fallback when no supported language is available.
func applicationLanguage(preference string) string {
	if preference == "" {
		return "zh-CN"
	}
	if preference != "system" {
		return preference
	}
	locales, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return "en-US"
	}
	return supportedLanguage(locales)
}

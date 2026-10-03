//go:build windows

package main

import "testing"

func TestLanguageSelectionRespectsSupportedPreferenceOrder(t *testing.T) {
	for _, test := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"fr-FR", "zh-Hans-CN", "en-US"}, "zh-CN"},
		{[]string{"en-GB", "zh-CN"}, "en-US"},
		{[]string{"ja-JP"}, "en-US"},
	} {
		if got := supportedLanguage(test.preferred); got != test.want {
			t.Fatalf("%v: %s", test.preferred, got)
		}
	}
}

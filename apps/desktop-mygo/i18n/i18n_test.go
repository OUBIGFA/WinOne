package i18n

import (
	"strings"
	"testing"
)

func TestTranslateFallsBackInterpolatesAndPluralizes(t *testing.T) {
	if got := Translate(Chinese, "nav.overview"); got != "首页" {
		t.Fatalf("zh overview = %q", got)
	}
	if got := Translate(English, "nav.overview"); got != "Home" {
		t.Fatalf("en overview = %q", got)
	}
	// Only English defines this key; Chinese falls back as i18next did.
	if got := Translate(Chinese, "wintray.minimized"); got != Translate(English, "wintray.minimized") || got == "wintray.minimized" {
		t.Fatalf("fallback = %q", got)
	}
	if got := Translate(Chinese, "wintray.stopTitle", "name", "记事本"); got != "关闭 记事本？" {
		t.Fatalf("interpolation = %q", got)
	}
	one, other := Translate(English, "aot.pinned", "count", 1), Translate(English, "aot.pinned", "count", 3)
	if one == other || one == "aot.pinned" {
		t.Fatalf("plural forms %q %q", one, other)
	}
	if got := Translate(Chinese, "missing.key"); got != "missing.key" {
		t.Fatalf("missing = %q", got)
	}
	if got := Translate(English, "x", "name", "v"); got != "x" {
		t.Fatalf("missing with args = %q", got)
	}
}

func TestResolveFollowsPreferenceThenHost(t *testing.T) {
	for _, c := range []struct{ preference, host, want string }{
		{"", "en-US", Chinese}, {"zh-CN", "en-US", Chinese}, {"en-US", "zh-CN", English},
		{"system", "zh-CN", Chinese}, {"system", "en-US", English}, {"system", "", English},
	} {
		if got := Resolve(c.preference, c.host); got != c.want {
			t.Errorf("Resolve(%q,%q) = %q, want %q", c.preference, c.host, got, c.want)
		}
	}
}

func TestBothLanguagesDefineTheSameKeys(t *testing.T) {
	for key := range tables[Chinese] {
		if _, ok := tables[English][key]; !ok {
			if _, plural := tables[English][key+"_other"]; !plural {
				t.Errorf("%s is missing in English", key)
			}
		}
	}
	for key := range tables[English] {
		if _, ok := tables[Chinese][key]; ok {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(key, "_one"), "_other")
		if _, ok := tables[Chinese][base]; !ok && key != "wintray.minimized" {
			t.Errorf("%s is missing in Chinese", key)
		}
	}
}

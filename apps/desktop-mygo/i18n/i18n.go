// Package i18n holds the desktop window's Simplified Chinese and English
// text. The tables were converted from the React frontend's i18next
// resources (apps/desktop/frontend/src/i18n.ts and locales/) with the same
// flattened keys, so every page keeps its existing wording.
//
// Semantics follow the i18next configuration it replaces: a missing key in
// the current language falls back to en-US, then to the key itself;
// "{{name}}" placeholders take named values; a "count" value selects the
// "_one" or "_other" form where a language provides them.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	Chinese = "zh-CN"
	English = "en-US"
)

//go:embed *.json
var files embed.FS

var tables = map[string]map[string]string{Chinese: load(Chinese), English: load(English)}

// current is read by views on the main thread and written by the shell when
// the language setting changes; atomic keeps background goroutines that
// format messages consistent with it.
var current atomic.Value

func init() { current.Store(Chinese) }

// load merges the language's table: <locale>.json, converted from the
// React window, and <page>.<locale>.json files that pages added since, one
// per page so that pages developed side by side never edit one file. A
// key defined twice is a mistake, not an override.
func load(locale string) map[string]string {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	table := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if name != locale+".json" && !strings.HasSuffix(name, "."+locale+".json") {
			continue
		}
		data, err := files.ReadFile(name)
		if err != nil {
			panic(err)
		}
		var part map[string]string
		if err := json.Unmarshal(data, &part); err != nil {
			panic(fmt.Errorf("i18n %s: %w", name, err))
		}
		for key, text := range part {
			if _, ok := table[key]; ok {
				panic(fmt.Errorf("i18n %s: %s is defined twice", name, key))
			}
			table[key] = text
		}
	}
	return table
}

// Resolve returns the language a preference selects: "system" follows the
// Host's locale (which already maps the Windows UI language), an empty
// preference is the product default, Chinese. Anything else is a language
// the window does not have, which i18next resolved to its en-US fallback.
func Resolve(preference, hostLocale string) string {
	switch preference {
	case English, Chinese:
		return preference
	case "":
		return Chinese
	case "system":
		if hostLocale == Chinese {
			return Chinese
		}
	}
	return English
}

// SetLocale selects the language of later T calls.
func SetLocale(locale string) {
	if locale != English {
		locale = Chinese
	}
	current.Store(locale)
}

// Locale returns the selected language.
func Locale() string { return current.Load().(string) }

// T returns the text of key in the current language. args are name/value
// pairs for "{{name}}" placeholders: T("wintray.stopTitle", "name", app).
func T(key string, args ...any) string { return Translate(Locale(), key, args...) }

// Translate is T for an explicit language.
func Translate(locale, key string, args ...any) string {
	values := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		values[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
	}
	text, ok := lookup(locale, key, values)
	if !ok && locale != English {
		text, ok = lookup(English, key, values)
	}
	if !ok {
		return key
	}
	return interpolate(text, values)
}

func lookup(locale, key string, values map[string]string) (string, bool) {
	table := tables[locale]
	if count, ok := values["count"]; ok {
		form := key + "_other"
		if count == "1" {
			form = key + "_one"
		}
		if text, ok := table[form]; ok {
			return text, true
		}
	}
	text, ok := table[key]
	return text, ok
}

func interpolate(text string, values map[string]string) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	var out strings.Builder
	for {
		start := strings.Index(text, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "}}")
		if end < 0 {
			break
		}
		name := strings.TrimSpace(text[start+2 : start+end])
		out.WriteString(text[:start])
		if value, ok := values[name]; ok {
			out.WriteString(value)
		} else {
			// i18next leaves an unknown placeholder as written.
			out.WriteString(text[start : start+end+2])
		}
		text = text[start+end+2:]
	}
	out.WriteString(text)
	return out.String()
}

// Has reports whether either language defines key, for keys composed from
// data such as module identifiers.
func Has(key string) bool {
	_, zh := tables[Chinese][key]
	_, en := tables[English][key]
	return zh || en
}

package aiservices

// Magpie's form helpers (github.com/yetone/magpie, MIT; notice in
// third_party/licenses/upstream-magpie-42ec2700bd2a4cf8d4ceb76a95767ac221e283e5-0-LICENSE.txt)
// keep URL escaping and header-row behavior used
// by the retained provider and gateway forms. Values pasted and saved here
// therefore follow Magpie's request semantics without importing its removed
// service pages.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// curlImport is the original Magpie paste parser's reply. Native controls
// only bind its values to the current draft; they do not parse shell text.
type curlImport struct {
	URL, API, Base, URLHole, Key, Placeholder, Model, Name string
	Bases                                                  [][2]string
}

// proxyParts is a proxy address split for the form: the address without
// the login, the login decoded, and the login as escaped in the address.
type proxyParts struct {
	Address, Username, Password string
	// EscapedUser and EscapedPassword keep a login as loaded, so a field
	// left alone keeps its escapes, even ones that are not UTF-8.
	EscapedUser, EscapedPassword string
}

var (
	proxySplit  = regexp.MustCompile(`(?is)^([a-z][\w+.-]*://)?([^/?#]*)(.*)$`)
	proxyScheme = regexp.MustCompile(`(?is)^([a-z][\w+.-]*://)?(.*)$`)
)

func proxyFields(value string) proxyParts {
	m := proxySplit.FindStringSubmatch(strings.TrimSpace(value))
	scheme, auth, rest := m[1], m[2], m[3]
	at := strings.LastIndex(auth, "@")
	info := ""
	if at >= 0 {
		info = auth[:at]
	}
	eu, ep := info, ""
	if c := strings.Index(info, ":"); c >= 0 {
		eu, ep = info[:c], info[c+1:]
	}
	return proxyParts{Address: scheme + auth[at+1:] + rest, Username: decodeURIComponent(eu), Password: decodeURIComponent(ep),
		EscapedUser: eu, EscapedPassword: ep}
}

// proxyAddress puts a login back into the address: each typed part
// percent-encoded, one left as loaded with its escapes.
func proxyAddress(address string, f proxyParts, user, pass string) (string, bool) {
	a := strings.TrimSpace(address)
	if a == "" || (user == "" && pass == "") {
		return a, true
	}
	enc := func(text, loaded, escaped string) (string, bool) {
		if text == loaded {
			return escaped, true
		}
		return encodeURIComponent(text)
	}
	m := proxyScheme.FindStringSubmatch(a)
	u, ok := enc(user, f.Username, f.EscapedUser)
	if !ok {
		return "", false
	}
	out := m[1] + u
	if pass != "" {
		p, ok := enc(pass, f.Password, f.EscapedPassword)
		if !ok {
			return "", false
		}
		out += ":" + p
	}
	return out + "@" + m[2], true
}

// decodeURIComponent is JavaScript's: text with an escape that is not
// UTF-8 or not an escape stays as it was.
func decodeURIComponent(v string) string {
	out, err := url.PathUnescape(v)
	if err != nil || !utf8.ValidString(out) {
		return v
	}
	return out
}

// encodeURIComponent is JavaScript's; text that is not UTF-8 fails, as
// JavaScript's fails on a lone surrogate.
func encodeURIComponent(text string) (string, bool) {
	if !utf8.ValidString(text) {
		return "", false
	}
	const unreserved = "-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String(), true
}

// headerRow is one request header as edited: name, value, and whether the
// value is edited as formatted JSON.
type headerRow struct {
	Name, Value string
	JSON        bool
}

// headerRows lists saved headers by name: a map has no order of its own.
func headerRows(headers map[string]string) []headerRow {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]headerRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, headerRow{Name: name, Value: headers[name]})
	}
	return rows
}

func looksJSON(value string) bool {
	text := strings.TrimSpace(value)
	return strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")
}

func minifyJSON(value string) string {
	var b bytes.Buffer
	if !json.Valid([]byte(value)) || json.Compact(&b, []byte(value)) != nil {
		return value
	}
	return b.String()
}

func prettyJSON(value string) string {
	var b bytes.Buffer
	if !json.Valid([]byte(value)) || json.Indent(&b, []byte(value), "", "  ") != nil {
		return value
	}
	return b.String()
}

// headersOf is Magpie's serialization: trimmed names and values, empty
// rows left out, a later name replacing an earlier one in any case, JSON
// values minified.
func headersOf(rows []headerRow) map[string]string {
	result := map[string]string{}
	for _, row := range rows {
		name, text := strings.TrimSpace(row.Name), strings.TrimSpace(row.Value)
		if name == "" || text == "" {
			continue
		}
		for previous := range result {
			if strings.EqualFold(previous, name) {
				delete(result, previous)
			}
		}
		if looksJSON(text) {
			text = minifyJSON(text)
		}
		result[name] = text
	}
	return result
}

// splitList reads a comma, Chinese comma or line separated list, trimmed,
// without empty entries.
func splitList(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '，' || r == '\n' })
	out := []string{}
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

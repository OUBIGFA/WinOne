package aiservices

// Magpie's form helpers (github.com/yetone/magpie, MIT; notice in
// third_party/licenses/upstream-magpie-42ec2700bd2a4cf8d4ceb76a95767ac221e283e5-0-LICENSE.txt),
// translated from the React window's extracts: curlOf and respellURL
// (features/magpie-curl.js, Magpie 7660399), proxyFields and proxyAddress
// (features/magpie-proxy-fields.ts, Magpie 42ec270), and the header rows
// (features/magpie-headers.ts, Magpie a8ca790). They keep the upstream
// parsing and escaping, so values pasted and saved here match Magpie's.

import (
	"bytes"
	"encoding/json"
	"net/url"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// curlImport is what a pasted cURL example names: the API it calls, its
// base address, a placeholder in that address, a key or a placeholder for
// one, and the model of its body. Placeholders never become saved keys.
type curlImport struct {
	URL, API, Base, URLHole, Key, Placeholder, Model string
}

var (
	curlHead       = regexp.MustCompile(`(?i)^curl(\.exe)?`)
	curlCmdBreak   = regexp.MustCompile(`\^[ \t]*\r?\n`)
	curlPSBreak    = regexp.MustCompile("`[ \\t]*\\r?\\n")
	curlPSExe      = regexp.MustCompile(`(?i)^curl\.exe`)
	curlBreaks     = regexp.MustCompile("(?:\\\\|\\^|`)[ \\t]*\\r?\\n")
	curlValued     = regexp.MustCompile(`^(-[HdXuoAeFbcmT]|--(header|data|data-raw|data-binary|data-ascii|data-urlencode|json|request|url|user|output|user-agent|referer|form|cookie|max-time|connect-timeout|proxy|upload-file|retry|cert|key|cacert))$`)
	curlInline     = regexp.MustCompile(`^-[Hd].`)
	curlData       = regexp.MustCompile(`^(-d|--data(-raw|-binary|-ascii)?|--json)$`)
	curlEndpoint   = regexp.MustCompile(`(?i)^(https?://[^?#]*?)/*(/chat/completions|/messages|/responses)/*(?:[?#].*)?$`)
	curlHole       = regexp.MustCompile(`\{[^}]*\}|<[^>]*>|\$\{?\w+\}?|%\w+%`)
	curlKeyHeader  = regexp.MustCompile(`(?i)^\s*(authorization|x-api-key|api-key)\s*:\s*(.*?)\s*$`)
	curlBearer     = regexp.MustCompile(`(?i)^bearer\s+`)
	curlPlaceholder = regexp.MustCompile(`(?i)^\$|^%\w+%$|^<.*>$|^\{\{.*\}\}$|^\[.*\]$|your[_-]?|api[_-]?key|x{6,}|\.\.\.|…`)
)

// curlOf reads a cURL command as the shell hands its words to curl:
// POSIX, cmd.exe and PowerShell quoting and line continuations. Other text
// returns nil, so an ordinary paste stays a paste.
func curlOf(text string) *curlImport {
	s := strings.TrimSpace(text)
	head := curlHead.FindString(s)
	if head == "" {
		return nil
	}
	if next, _ := utf8.DecodeRuneInString(s[len(head):]); !isJSSpace(next) {
		return nil
	}
	cmd := curlCmdBreak.MatchString(s)
	ps := curlPSBreak.MatchString(s) || curlPSExe.MatchString(s)
	s = curlBreaks.ReplaceAllString(s[len(head):], " ")

	// The words as the shell hands them to curl: quotes taken off, escapes
	// read.
	r := []rune(s)
	var words []string
	var w *strings.Builder
	at := func(i int) rune {
		if i < len(r) {
			return r[i]
		}
		return 0
	}
	for i := 0; i < len(r); {
		c := r[i]
		if isJSSpace(c) {
			if w != nil {
				words = append(words, w.String())
			}
			w = nil
			i++
			continue
		}
		if w == nil {
			w = &strings.Builder{}
		}
		switch {
		case c == '\'':
			// PowerShell doubles a quote inside single quotes.
			j := i + 1
			for {
				k := indexRune(r, '\'', j)
				if k < 0 {
					w.WriteString(string(r[j:]))
					i = len(r)
					break
				}
				if ps && at(k+1) == '\'' {
					w.WriteString(string(r[j:k]) + "'")
					j = k + 2
					continue
				}
				w.WriteString(string(r[j:k]))
				i = k + 1
				break
			}
		case c == '"':
			for i++; i < len(r) && r[i] != '"'; i++ {
				escaped := (r[i] == '\\' && strings.ContainsRune("\"\\$`", at(i+1)) && at(i+1) != 0) || (ps && r[i] == '`')
				if escaped {
					i++
				}
				if i < len(r) {
					w.WriteRune(r[i])
				}
			}
			i++
		case (c == '\\' && !cmd && !ps) || (c == '^' && cmd):
			if i+1 < len(r) {
				w.WriteRune(r[i+1])
			}
			i += 2
		default:
			w.WriteRune(c)
			i++
		}
	}
	if w != nil {
		words = append(words, w.String())
	}

	// The options curl reads a value after; the others stand alone.
	var address, body string
	var headers []string
	for i := 0; i < len(words); i++ {
		x := words[i]
		opt, val := x, ""
		switch {
		case curlInline.MatchString(x):
			opt, val = x[:2], x[2:]
		case curlValued.MatchString(x):
			i++
			if i < len(words) {
				val = words[i]
			}
		case strings.HasPrefix(x, "-"):
			continue
		default:
			if address == "" {
				address = x
			}
			continue
		}
		switch {
		case opt == "-H" || opt == "--header":
			headers = append(headers, val)
		case opt == "--url":
			address = val
		case curlData.MatchString(opt) && !strings.HasPrefix(val, "@"):
			body = val
		}
	}
	out := &curlImport{URL: address}
	if m := curlEndpoint.FindStringSubmatch(address); m != nil {
		out.API = map[string]string{"/chat/completions": "openai", "/messages": "anthropic", "/responses": "responses"}[strings.ToLower(m[2])]
		out.Base = respellURL(m[1], out.API)
		out.URLHole = curlHole.FindString(out.Base)
	}
	for _, h := range headers {
		m := curlKeyHeader.FindStringSubmatch(h)
		if m == nil {
			continue
		}
		v := m[2]
		if strings.ToLower(m[1]) == "authorization" {
			v = curlBearer.ReplaceAllString(v, "")
		}
		if v == "" {
			continue
		}
		if curlPlaceholder.MatchString(v) {
			out.Placeholder = v
		} else {
			out.Key = v
		}
		break
	}
	// A body for PowerShell 5 often escapes its quotes for curl.exe:
	// '{\"model\": …}'.
	for _, b := range []string{body, strings.ReplaceAll(body, `\"`, `"`)} {
		var j any
		if json.Unmarshal([]byte(b), &j) != nil {
			continue
		}
		if object, ok := j.(map[string]any); ok {
			if model, ok := object["model"].(string); ok {
				out.Model = strings.TrimSpace(model)
			}
		}
		break
	}
	return out
}

func indexRune(r []rune, c rune, from int) int {
	for i := from; i < len(r); i++ {
		if r[i] == c {
			return i
		}
	}
	return -1
}

// isJSSpace is JavaScript's \s: Unicode white space and the byte order mark.
func isJSSpace(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' }

var (
	trailingSlashes = regexp.MustCompile(`/+$`)
	anthropicV1     = regexp.MustCompile(`/v1$`)
	systemOne       = regexp.MustCompile(`/systemone$`)
	hostV1          = regexp.MustCompile(`^https?://[^/]+/v1$`)
	hostOnly        = regexp.MustCompile(`^https?://[^/]+$`)
	versionSuffix   = regexp.MustCompile(`/v\d+[a-z]*$`)
)

// respellURL turns an endpoint's base into the address Magpie saves for
// its API.
func respellURL(u, api string) string {
	u = trailingSlashes.ReplaceAllString(strings.TrimSpace(u), "")
	switch api {
	case "anthropic":
		return anthropicV1.ReplaceAllString(u, "")
	case "decide":
		u = systemOne.ReplaceAllString(u, "")
	case "gemini":
		switch {
		case hostV1.MatchString(u):
			return u + "beta"
		case hostOnly.MatchString(u):
			return u + "/v1beta"
		}
		return u
	}
	if versionSuffix.MatchString(u) || !hostOnly.MatchString(u) {
		return u
	}
	return u + "/v1"
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

var (
	ruleSeparators = regexp.MustCompile(`[ .-]`)
	ruleOther      = regexp.MustCompile(`[^A-Z0-9_]`)
	ruleEdges      = regexp.MustCompile(`^_+|_+$`)
)

// ruleKind normalizes a masking rule's kind as Magpie's redact.RuleKind.
func ruleKind(kind string) string {
	k := strings.ToUpper(strings.TrimSpace(kind))
	k = ruleSeparators.ReplaceAllString(k, "_")
	k = ruleOther.ReplaceAllString(k, "")
	k = ruleEdges.ReplaceAllString(k, "")
	if k == "" {
		return "CUSTOM"
	}
	return k
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

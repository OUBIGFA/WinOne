package resultui

// Inline token rules adapted from Cindy thinkingText.ts, Copyright 2026 XD
// Inc., Apache-2.0. The native result host keeps the same narrow presentation:
// strong and inline code only; headings, links, images and HTML remain text.
import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type thinkingToken struct{ Kind, Value string }
type thinkingDocument struct {
	raw    string
	tokens []thinkingToken
}

var thinkingNewlines = regexp.MustCompile(`[\r\n]+`)

func thinkingEscaped(text string, index int) bool {
	slashes := 0
	for index--; index >= 0 && text[index] == '\\'; index-- {
		slashes++
	}
	return slashes%2 == 1
}
func whitespaceBefore(text string, index int) bool {
	if index <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return unicode.IsSpace(r)
}
func whitespaceAt(text string, index int) bool {
	if index >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return unicode.IsSpace(r)
}
func thinkingTokens(text string) []thinkingToken {
	var tokens []thinkingToken
	plain, cursor := 0, 0
	flush := func(end int) {
		if end > plain {
			tokens = append(tokens, thinkingToken{"text", text[plain:end]})
		}
	}
	for cursor < len(text) {
		if text[cursor] == '`' && !thinkingEscaped(text, cursor) {
			run := 1
			for cursor+run < len(text) && text[cursor+run] == '`' {
				run++
			}
			start := cursor + run
			delimiter := strings.Repeat("`", run)
			close := -1
			for scan := start; scan < len(text); {
				offset := strings.Index(text[scan:], delimiter)
				if offset < 0 {
					break
				}
				at := scan + offset
				if !thinkingEscaped(text, at) && (at == 0 || text[at-1] != '`') && (at+run == len(text) || text[at+run] != '`') {
					close = at
					break
				}
				scan = at + run
			}
			if close >= 0 {
				flush(cursor)
				value := thinkingNewlines.ReplaceAllString(text[start:close], " ")
				if len(value) >= 2 && strings.HasPrefix(value, " ") && strings.HasSuffix(value, " ") && strings.TrimSpace(value) != "" {
					value = value[1 : len(value)-1]
				}
				tokens = append(tokens, thinkingToken{"code", value})
				cursor = close + run
				plain = cursor
				continue
			}
			cursor += run
			continue
		}
		if strings.HasPrefix(text[cursor:], "**") && !thinkingEscaped(text, cursor) && (cursor == 0 || text[cursor-1] != '*') && cursor+2 < len(text) && text[cursor+2] != '*' && !whitespaceAt(text, cursor+2) {
			start := cursor + 2
			close := -1
			for scan := start; scan < len(text)-1; {
				offset := strings.Index(text[scan:], "**")
				if offset < 0 {
					break
				}
				at := scan + offset
				if at > start && !thinkingEscaped(text, at) && text[at-1] != '*' && !whitespaceBefore(text, at) && (at+2 == len(text) || text[at+2] != '*') {
					close = at
					break
				}
				scan = at + 2
			}
			if close >= 0 {
				flush(cursor)
				tokens = append(tokens, thinkingToken{"strong", text[start:close]})
				cursor = close + 2
				plain = cursor
				continue
			}
			cursor += 2
			continue
		}
		cursor++
	}
	flush(len(text))
	return tokens
}

func (m *Message) updateThinking(value string, answerStarted bool, now time.Time) {
	if m.Reasoning == "" && value != "" && !answerStarted {
		m.ReasoningStartedAt = now.UnixMilli()
		m.ReasoningActive = true
	}
	m.Reasoning = value
	if answerStarted {
		m.finishThinking(now)
	}
}
func (m *Message) finishThinking(now time.Time) {
	if !m.ReasoningActive {
		return
	}
	m.ReasoningActive = false
	m.ReasoningDurationMs = max(0, now.UnixMilli()-m.ReasoningStartedAt)
}
func thinkingDuration(ms int64) string {
	seconds := max(0, ms/1000)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds%60)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes%60)
	}
	return fmt.Sprintf("%dd %dh %dm", hours/24, hours%24, minutes%60)
}

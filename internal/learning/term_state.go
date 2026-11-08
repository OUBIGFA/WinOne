package learning

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"winone/internal/settings"
)

// TermState affects WinOne's reading list, independently of Anki scheduling.
type TermState string

const (
	Ignored TermState = "ignored"
	Focused TermState = "focused"
)

type TermStates map[string]TermState

// Exact normalized terms avoid guessing lemmas and hiding unrelated words.
func TermKey(term string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(term, "’", "'")), " "))
}
func (s TermStates) State(term string) TermState { return s[TermKey(term)] }

// Matching uses word boundaries, so focusing "art" does not match "partial".
func (s TermStates) Matching(text string, state TermState) []string {
	text = TermKey(text)
	var terms []string
	for term, value := range s {
		if value == state && regexp.MustCompile(`\b`+regexp.QuoteMeta(term)+`\b`).MatchString(text) {
			terms = append(terms, term)
		}
	}
	slices.Sort(terms)
	return terms
}

func LoadTermStates(path string) (TermStates, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return TermStates{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("学习词条状态文件过大")
	}
	var states TermStates
	if err = json.Unmarshal(data, &states); err != nil {
		return nil, err
	}
	if states == nil {
		states = TermStates{}
	}
	for term, state := range states {
		if term != TermKey(term) || !validTerm(term) || (state != Ignored && state != Focused) {
			return nil, errors.New("学习词条状态无效，请检查保存文件")
		}
	}
	return states, nil
}
func validTerm(term string) bool {
	kind := Classify(term)
	return len(term) <= 256 && (kind == Word || kind == Phrase)
}

// The Host serializes these read-modify-writes for all result windows. Return
// the new snapshot only after durable replacement; a failure preserves data.
func SetTermState(path, term string, state TermState) (TermStates, error) {
	term = TermKey(term)
	if !validTerm(term) || (state != "" && state != Ignored && state != Focused) {
		return nil, errors.New("词条或学习状态无效")
	}
	states, err := LoadTermStates(path)
	if err != nil {
		return nil, err
	}
	if state == "" {
		delete(states, term)
	} else {
		states[term] = state
	}
	data, err := json.Marshal(states)
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("学习词条状态已达容量限制")
	}
	if err = settings.WriteAtomic(path, data); err != nil {
		return nil, err
	}
	return states, nil
}

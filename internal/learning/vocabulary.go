package learning

import "strings"

// VocabularyNotes reads only explicitly labelled learning table columns. It
// does not infer notes from arbitrary prose, numeric tables or code blocks.
func VocabularyNotes(markdown string) []Note {
	var out []Note
	word, ipa, meaning := -1, -1, -1
	code := false
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			code = !code
			continue
		}
		if code {
			continue
		}
		if !strings.Contains(line, "|") {
			word, ipa, meaning = -1, -1, -1
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.Trim(strings.TrimSpace(cells[i]), "*`")
		}
		if word < 0 {
			for i, cell := range cells {
				switch strings.ToLower(cell) {
				case "word", "word / phrase", "词汇", "单词", "词条", "生词与短语":
					word = i
				case "ipa", "phonetic", "音标":
					ipa = i
				case "meaning", "释义", "含义":
					meaning = i
				}
			}
			if word < 0 || meaning < 0 {
				word, ipa, meaning = -1, -1, -1
			}
			continue
		}
		if word >= len(cells) || meaning >= len(cells) {
			continue
		}
		value := cells[word]
		if value == "" || strings.Trim(value, "-: ") == "" {
			continue
		}
		if kind := Classify(value); kind != Word && kind != Phrase {
			continue
		}
		note := Note{Word: value, Meaning: cells[meaning]}
		if ipa >= 0 && ipa < len(cells) {
			note.Phonetic = cells[ipa]
		}
		if note.Meaning != "" {
			out = append(out, note)
		}
	}
	return out
}

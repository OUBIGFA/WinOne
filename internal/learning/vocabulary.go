package learning

import "strings"

// VocabularyNotes reads only explicitly labelled learning table columns. It
// does not infer notes from arbitrary prose, numeric tables or code blocks.
func VocabularyNotes(markdown string) []Note {
	var out []Note
	word, ipa, meaning := -1, -1, -1
	var fence markdownFence
	columns := 0
	awaitingDelimiter := false
	for _, line := range strings.Split(markdown, "\n") {
		// Four-column indentation denotes code, even without an explicit fence.
		indent := 0
		for _, c := range line {
			if c == ' ' {
				indent++
			} else if c == '\t' {
				indent += 4 - indent%4
			} else {
				break
			}
		}
		if indent >= 4 {
			word, ipa, meaning = -1, -1, -1
			continue
		}
		line = strings.TrimSpace(line)
		if fence.consume(line) {
			word, ipa, meaning = -1, -1, -1
			continue
		}
		cells := tableCells(line)
		if len(cells) == 0 {
			word, ipa, meaning = -1, -1, -1
			continue
		}
		// A labelled line is only a header when the next row is the Markdown
		// delimiter. Otherwise prose containing pipes must not become cards.
		if word >= 0 && awaitingDelimiter {
			awaitingDelimiter = false
			valid := len(cells) == columns
			for _, cell := range cells {
				value := strings.Trim(cell, ":")
				valid = valid && value != "" && strings.Trim(value, "-") == ""
			}
			if valid {
				continue
			}
			word, ipa, meaning = -1, -1, -1
		}
		for i := range cells {
			cells[i] = strings.Trim(cells[i], "*`")
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
			columns, awaitingDelimiter = len(cells), true
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

// Markdown permits both fence characters and longer fences around examples
// that themselves contain triple backticks. Only a matching closing fence ends it.
type markdownFence struct {
	marker byte
	length int
}

func (f *markdownFence) consume(line string) bool {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return f.length > 0
	}
	n := 0
	for n < len(line) && line[n] == line[0] {
		n++
	}
	if f.length > 0 {
		if line[0] == f.marker && n >= f.length && strings.TrimSpace(line[n:]) == "" {
			f.length = 0
		}
		return true
	}
	if n < 3 || (line[0] == '`' && strings.Contains(line[n:], "`")) {
		return false
	}
	f.marker, f.length = line[0], n
	return true
}

// Escaped pipes are cell content, including inside inline code. Preserve other
// escapes; this helper only interprets the table's column boundaries.
func tableCells(line string) []string {
	var cells []string
	var cell strings.Builder
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '|':
			cell.WriteByte('|')
			i++
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '\\':
			cell.WriteString(line[i : i+2])
			i++
		case line[i] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(line[i])
		}
	}
	if len(cells) == 0 {
		return nil
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	if cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

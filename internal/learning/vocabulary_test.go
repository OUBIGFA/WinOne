package learning

import (
	"reflect"
	"testing"
)

func TestVocabularyNotesKeepsTableCellContents(t *testing.T) {
	for _, text := range []string{
		"| Word | IPA | Meaning |\n| --- | --- | --- |\n| alternate | /a\\|b/ | first \\| second |",
		"Word | IPA | Meaning\n--- | --- | ---\nalternate | /a\\|b/ | first \\| second",
	} {
		want := []Note{{Word: "alternate", Phonetic: "/a|b/", Meaning: "first | second"}}
		if got := VocabularyNotes(text); !reflect.DeepEqual(got, want) {
			t.Fatalf("VocabularyNotes(%q) = %#v, want %#v", text, got, want)
		}
	}
}

func TestVocabularyNotesIgnoresCodeExamplesAndNonTables(t *testing.T) {
	table := "| Word | Meaning |\n| --- | --- |\n| example | example only |\n"
	for name, text := range map[string]string{
		"indented code":      "    Word | Meaning\n    --- | ---\n    example | example only\n",
		"tabbed code":        "\tWord | Meaning\n\t--- | ---\n\texample | example only\n",
		"tilde fence":        "~~~markdown\n" + table + "~~~\n",
		"long fence":         "````markdown\n```\n" + table + "```\n````\n",
		"missing delimiter":  "Word | Meaning\nexample | example only\n",
		"header before code": "Word | Meaning\n--- | ---\n```\nignored\n```\nexample | example only\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := VocabularyNotes(text); len(got) != 0 {
				t.Fatalf("non-table content became notes: %#v", got)
			}
		})
	}
}

func TestVocabularyNotesResumesAfterCodeAndSeparatesTables(t *testing.T) {
	text := "~~~\nWord | Meaning\n--- | ---\nwrong | code\n~~~\n\n" +
		"| 单词 | 释义 |\n| :--- | ---: |\n| hello | 你好 |\n\n" +
		"Meaning | Word\n--- | ---\n世界 | world\n"
	want := []Note{{Word: "hello", Meaning: "你好"}, {Word: "world", Meaning: "世界"}}
	if got := VocabularyNotes(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("notes = %#v, want %#v", got, want)
	}
}

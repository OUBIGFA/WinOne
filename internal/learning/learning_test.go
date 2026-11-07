package learning

import (
	"strings"
	"testing"
)

func TestClassifyLearningContent(t *testing.T) {
	for _, tt := range []struct {
		text string
		kind Kind
	}{
		{"hello", Word}, {"state-of-the-art", Word}, {"don't", Word}, {"learner’s", Word},
		{"take care of", Phrase}, {"How are you?", Sentence}, {"你好", Sentence}, {"123", Sentence},
		{strings.Repeat("a", 46), Sentence}, {"first\nsecond", Sentence}, {strings.Repeat("x ", 301), Passage},
	} {
		if got := Classify(tt.text); got != tt.kind {
			t.Errorf("Classify(%q)=%s, want %s", tt.text, got, tt.kind)
		}
	}
}

func TestLearningOptionsAndFactGrounding(t *testing.T) {
	o := Defaults()
	if o.Level != "intermediate" || o.Accent != "us" || o.OnlineDictionary || !o.HumanVoice || o.Deck != "WinOne 生词本" {
		t.Fatalf("bad defaults: %+v", o)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []Kind{Word, Phrase, Sentence, Passage} {
		s := Prompt(o, kind, "Only describe verified roots.", "zh-CN")
		if !strings.Contains(s, "Only describe verified roots.") || !strings.Contains(s, "untrusted") {
			t.Fatal("custom instruction or source boundary missing")
		}
	}
	o.Level = "unknown"
	if o.Validate() == nil {
		t.Fatal("invalid level accepted")
	}
	o = Defaults()
	o.Deck = " "
	if o.Validate() == nil {
		t.Fatal("empty deck accepted")
	}
}

func TestExtractLearningFields(t *testing.T) {
	// Headings emitted by our templates map to distinct editable fields; prose
	// with no matching heading remains analysis and must not become a fake example.
	f := ExtractFields("## 核心释义\nv. 走\n## 例句\nI went home. 我回家了。\n## 词源与词根\n不规则过去式\n## 搭配\ngo home")
	if f.Meaning != "v. 走" || !strings.Contains(f.Example, "I went home") || f.Etymology != "不规则过去式" {
		t.Fatalf("%+v", f)
	}
	if f := ExtractFields("Unstructured answer"); f.Example != "" || f.Meaning != "" {
		t.Fatal("guessed structured fields")
	}
}

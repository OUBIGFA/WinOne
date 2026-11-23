package learning

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLearningOptionsRetireCardSettings(t *testing.T) {
	options := Defaults()
	options.Level, options.Accent, options.OnlineDictionary = "advanced", "uk", true
	options.Templates = map[Kind]string{Word: "My lesson"}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	if options.Level != "advanced" || options.Accent != "uk" || !options.OnlineDictionary || options.Templates[Word] != "My lesson" {
		t.Fatalf("learning preferences changed: %+v", options)
	}
	saved, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), `"deck"`) || strings.Contains(string(saved), `"cardMode"`) {
		t.Fatalf("retired card settings were saved: %s", saved)
	}
}

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
	if o.Level != "intermediate" || o.Accent != "us" || o.OnlineDictionary || !o.HumanVoice {
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
	o.Accent = "unknown"
	if o.Validate() == nil {
		t.Fatal("invalid accent accepted")
	}
}

func TestExtractLearningMeaning(t *testing.T) {
	meaning := ExtractMeaning("## 核心释义\nv. 走\n## 例句\nI went home. 我回家了。\n## 词源与词根\n不规则过去式\n## 搭配\ngo home")
	if meaning != "v. 走" {
		t.Fatalf("%q", meaning)
	}
	if ExtractMeaning("Unstructured answer") != "" {
		t.Fatal("guessed meaning")
	}
}

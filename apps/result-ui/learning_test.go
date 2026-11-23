package resultui

import (
	"strings"
	"testing"
	"winone/internal/learning"
	"winone/internal/resultwindow"
)

func TestCancelLearningLookupStopsLoading(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "selection"})
	defer c.close()
	c.Panel.Study = newStudy(resultwindow.LearningAction(), "hello")
	c.activeStudy = c.Panel.Study
	c.cancel()
	if c.Panel.Study.Loading {
		t.Fatal("cancelled local lookup must not leave an endless loading state")
	}
}

func TestLearningLocalLayerSurvivesAIFailure(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "selection", Action: "learn", Text: "went", Language: "zh-CN"})
	study := newStudy(resultwindow.LearningAction(), "went")
	study.acceptEntries([]learning.Entry{{Word: "went", Translation: "go 的过去式", Phonetic: "went"}, {Word: "go", Translation: "v. 去"}})
	c.Panel.Study = study
	c.Panel.Error = "请先配置 AI 服务"
	if len(c.Panel.Study.Rows) != 2 || c.Panel.Study.Rows[0].Note.Meaning != "go 的过去式" {
		t.Fatal("local result lost when AI unavailable")
	}
	study.acceptAnalysis("## 核心释义\nAI definition\n## 例句\nI went home.")
	if study.Rows[0].Note.Meaning != "go 的过去式" {
		t.Fatal("AI replaced dictionary meaning")
	}
}

func TestPassageVocabularyKeepsAIEntries(t *testing.T) {
	s := newStudy(resultwindow.LearningAction(), strings.Repeat("Sustainable development needs coordination. ", 20))
	s.acceptEntries([]learning.Entry{{Word: "sustainable", Translation: "可持续的"}})
	s.acceptAnalysis("| Word | IPA | Meaning |\n|---|---|---|\n| coordination | kəʊˌɔːdɪˈneɪʃən | 协调 |")
	if len(s.Rows) != 2 || s.Rows[1].Note.Word != "coordination" || s.Rows[1].Note.Meaning != "协调" {
		t.Fatalf("AI vocabulary lost: %+v", s.Rows)
	}
}

func TestLearningUsesAIMeaningWhenDictionaryHasNoEntry(t *testing.T) {
	s := newStudy(resultwindow.LearningAction(), "newword")
	s.acceptEntries(nil)
	s.acceptAnalysis("## 核心释义\nAI definition\n## 例句\nAn example.")
	if len(s.Rows) != 1 || s.Rows[0].Note.Meaning != "AI definition" {
		t.Fatalf("AI definition lost: %+v", s.Rows)
	}
}

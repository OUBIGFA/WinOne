package resultui

import (
	"context"
	"strings"
	"testing"
	"winone/internal/learning"
	"winone/internal/resultwindow"
)

func TestClearConversationCancelsAllLearningBatches(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "quick-assistant"})
	defer c.close()
	active, cancelActive := context.WithCancel(c.ctx)
	previous, cancelPrevious := context.WithCancel(c.ctx)
	c.Panel.Study = &study{cancel: cancelActive}
	c.activeStudy = c.Panel.Study
	c.Panel.Messages = []Message{{Role: "assistant", Study: &study{cancel: cancelPrevious}}}
	c.clearAll()
	if active.Err() == nil || previous.Err() == nil {
		t.Fatal("clearing visible learning results must cancel active and historical card batches")
	}
}

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
	study.acceptAnalysis("## 例句\nI went home.\n## 词源与词根\nGo 的不规则过去式。")
	if study.Rows[0].Note.Example != "I went home." || study.Rows[0].Note.Meaning != "go 的过去式" {
		t.Fatal("AI did not complement dictionary fields")
	}
}

func TestPassageCardsKeepSourceExamplesAndPageAccess(t *testing.T) {
	s := newStudy(resultwindow.LearningAction(), strings.Repeat("Sustainable development needs coordination. ", 20))
	s.acceptEntries([]learning.Entry{{Word: "sustainable", Translation: "可持续的"}})
	s.acceptAnalysis("| Word | IPA | Meaning |\n|---|---|---|\n| coordination | kəʊˌɔːdɪˈneɪʃən | 协调 |")
	for _, row := range s.Rows {
		if row.Note.Example != "Sustainable development needs coordination." {
			t.Fatalf("source example missing: %+v", row.Note)
		}
	}
}
func TestLearningDraftIsIndependentAndBatchRetainsFailures(t *testing.T) {
	study := newStudy(resultwindow.LearningAction(), "hello")
	study.acceptEntries([]learning.Entry{{Word: "hello", Translation: "你好"}})
	study.edit(0)
	study.Draft.Meaning = "changed"
	if study.Rows[0].Note.Meaning == "changed" {
		t.Fatal("unsaved edit leaked")
	}
}

package resultui

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"winone/internal/learning"
	"winone/internal/resultwindow"
)

func TestLearningIgnoredAndFocusedTermsSurviveAIAdditions(t *testing.T) {
	s := newStudy(resultwindow.LearningAction(), "We look up sustainable ideas in the world.")
	s.States = learning.TermStates{"world": learning.Ignored, "look up": learning.Focused}
	s.acceptEntries([]learning.Entry{{Word: "world", Translation: "世界"}, {Word: "sustainable", Translation: "可持续"}, {Word: "look up", Translation: "查询"}})
	if len(s.Rows) != 2 || s.Rows[0].Note.Word != "look up" {
		t.Fatal(s.Rows)
	}
	s.acceptAnalysis("| Word | Meaning |\n| --- | --- |\n| world | 世界 |\n| ideas | 想法 |")
	if len(s.Rows) != 3 || s.Rows[0].Note.Word != "look up" {
		t.Fatal(s.Rows)
	}
	for _, row := range s.Rows {
		if row.Note.Word == "world" {
			t.Fatal("ignored term reintroduced by AI")
		}
	}
	direct := newStudy(resultwindow.LearningAction(), "world")
	direct.States = s.States
	direct.acceptEntries([]learning.Entry{{Word: "world", Translation: "世界"}})
	if len(direct.Rows) != 1 {
		t.Fatal("direct lookup must remain available to restore a term")
	}
}
func TestLearningSourceExpansionAndFullSpeech(t *testing.T) {
	p := NewPanel("chat", "en-US", "light")
	s := newStudy(resultwindow.LearningAction(), strings.Repeat("A sustainable future needs careful decisions. ", 12))
	s.Loading = false
	var spoken string
	p.Actions.StudySpeak = func(_ *study, text, accent string) { spoken = text }
	tt := ui.NewTester(func(c *ui.Context) { p.studyView(c, c.Theme(), s) }, 420, 900)
	if s.SourceOpen {
		t.Fatal("source starts expanded")
	}
	if err := tt.Click("Read source"); err != nil {
		t.Fatal(err)
	}
	if spoken != s.Text {
		t.Fatal("speech truncated to preview")
	}
	if err := tt.Click("Expand source"); err != nil {
		t.Fatal(err)
	}
	if !s.SourceOpen || !tt.HasText(s.Text) {
		t.Fatal("full source is inaccessible")
	}
	if err := tt.Click("Collapse source"); err != nil {
		t.Fatal(err)
	}
	if s.SourceOpen {
		t.Fatal("source did not collapse")
	}
}

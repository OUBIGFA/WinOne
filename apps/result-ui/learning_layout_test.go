package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/learning"
	"winone/internal/resultwindow"
)

func TestLearningLayoutAndNoteEditor(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en-US"} {
		for _, theme := range []string{"light", "dark"} {
			p := NewPanel("translate", lang, theme)
			p.Source = "selection"
			p.Action = "learn"
			p.Study = newStudy(resultwindow.LearningAction(), "sustainable")
			p.Study.acceptEntries([]learning.Entry{{Word: "sustainable", Phonetic: "səˈsteɪnəbl", Translation: "a. 可持续的；能维持的", Definition: "able to continue over a period of time", Tags: []string{"六级", "考研", "雅思"}, COCA: 7890, BNC: 9000, Collins: 2}})
			p.Study.Connected = true
			p.Result = "## 核心释义\n能够持续维持，不耗尽资源。\n## 例句\nWe need sustainable solutions. 我们需要可持续的解决方案。"
			p.Actions.StudyAdd = func(s *study, indices []int) {
				if len(indices) != 1 || s.Rows[indices[0]].Note.Meaning == "" {
					t.Error("incorrect edited note")
				}
				s.Rows[indices[0]].Added = true
			}
			if folder := os.Getenv("WINONE_LEARNING_SHOTS"); folder != "" {
				for _, width := range []int{320, 420, 800} {
					tt := ui.NewTester(p.View, width, 900)
					if err := os.MkdirAll(folder, 0700); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(folder, fmt.Sprintf("reading-%s-%s-%d.png", lang, theme, width)))
					if err != nil {
						t.Fatal(err)
					}
					if err = png.Encode(file, tt.Image()); err != nil {
						t.Fatal(err)
					}
					file.Close()
				}
			}
			tt := ui.NewTester(p.View, 420, 900)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
			if err := tt.Click(p.T("加入生词本", "Add to vocabulary")); err != nil {
				t.Fatal(err)
			}
			if p.Study.Draft == nil {
				t.Fatal("note editor did not open")
			}
			for _, width := range []int{320, 420, 800} {
				tt.SetSize(width, 900)
				tt.Frame()
				if box, ok := tt.Find(p.T("继续提问", "Ask a follow-up")); !ok || box.X < 0 || box.X+box.W > float32(width) {
					t.Fatalf("composer inaccessible at %d: %+v", width, box)
				}
				if folder := os.Getenv("WINONE_LEARNING_SHOTS"); folder != "" {
					if err := os.MkdirAll(folder, 0700); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(folder, fmt.Sprintf("learning-%s-%s-%d.png", lang, theme, width)))
					if err != nil {
						t.Fatal(err)
					}
					if err = png.Encode(file, tt.Image()); err != nil {
						t.Fatal(err)
					}
					file.Close()
				}
			}
		}
	}
}

func TestPassagePagingKeepsAllCardsAvailable(t *testing.T) {
	p := NewPanel("chat", "en-US", "light")
	s := newStudy(resultwindow.LearningAction(), strings.Repeat("A longer passage. ", 50))
	var entries []learning.Entry
	for i := 0; i < 25; i++ {
		entries = append(entries, learning.Entry{Word: fmt.Sprintf("entry-%d", i), Translation: "A useful meaning", Definition: "The full dictionary definition"})
	}
	s.acceptEntries(entries)
	s.Connected = true
	p.Actions.StudyAdd = func(_ *study, indices []int) {
		if len(indices) != 25 {
			t.Fatalf("Add all omitted another page: %d", len(indices))
		}
	}
	tt := ui.NewTester(func(c *ui.Context) { p.studyView(c, c.Theme(), s) }, 420, 5000)
	if !tt.HasText("entry-0") || tt.HasText("entry-12") {
		t.Fatal("first page is incorrect")
	}
	if tt.HasText("The full dictionary definition") {
		t.Fatal("compact list expanded all details")
	}
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("entry-12") || tt.HasText("entry-0") {
		t.Fatal("second page is inaccessible")
	}
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("entry-24") {
		t.Fatal("last row is inaccessible")
	}
	if err := tt.Click("Show details"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("The full dictionary definition") {
		t.Fatal("details not expanded")
	}
	if err := tt.Click("Vocabulary and study tools"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300000000)
	if err := tt.Click("Add all"); err != nil {
		t.Fatal(err)
	}
}

func TestSentenceLearningScreensAndStateActions(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en-US"} {
		for _, theme := range []string{"light", "dark"} {
			p := NewPanel("translate", lang, theme)
			p.Source = "selection"
			p.Action = "learn"
			s := newStudy(resultwindow.LearningAction(), "We look up sustainable solutions and put them into practice every day.")
			s.States = learning.TermStates{"look up": learning.Focused, "every day": learning.Ignored}
			s.acceptEntries([]learning.Entry{{Word: "sustainable", Translation: "a. 可持续的；能维持的", Phonetic: "səˈsteɪnəbl"}, {Word: "look up", Translation: "查阅；查找"}, {Word: "put into practice", Translation: "付诸实践"}})
			s.Connected = true
			p.Study = s
			p.Actions.StudyState = func(s *study, word string, state learning.TermState) {
				if state == "" {
					delete(s.States, learning.TermKey(word))
				} else {
					s.States[learning.TermKey(word)] = state
				}
			}
			for _, width := range []int{320, 420, 800} {
				for _, manage := range []bool{false, true} {
					s.StatesOpen = manage
					tt := ui.NewTester(p.View, width, 1000)
					tt.Advance(300000000)
					for _, text := range tt.Texts() {
						if r, ok := tt.Find(text); ok && (r.X < -.5 || r.X+r.W > float32(width)+.5) {
							t.Fatalf("learning text overflow %d %q: %+v", width, text, r)
						}
					}
					if folder := os.Getenv("WINONE_LEARNING_SHOTS"); folder != "" {
						file, err := os.Create(filepath.Join(folder, fmt.Sprintf("sentence-%s-%s-%d-%t.png", lang, theme, width, manage)))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tt.Image())
						file.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
	}
}

//go:build windows

package ui

import "testing"

func TestWinOneStandardCopyReadsSelectableParagraphs(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Selectable().Padding(12).Gap(8).Children(func() {
			coreText(c, "Before selected")
			coreText(c, "second after")
		})
	}, 320, 160)
	selectBetween(t, tt, "Before selected", 7, "second after", 6)
	tt.SetClipboard("original clipboard")
	tt.Key(Ctrl, KeyInsert)
	if got := tt.Clipboard(); got != "selected\nsecond" {
		t.Fatalf("standard copy lost the actual multiline selection: %q", got)
	}
	tt.SetClipboard("check existing shortcut")
	tt.Key(Ctrl, KeyC)
	if got := tt.Clipboard(); got != "selected\nsecond" {
		t.Fatalf("existing copy shortcut regressed: %q", got)
	}
}

func TestWinOneStandardCopyKeepsInputAndPasswordProtection(t *testing.T) {
	for _, password := range []bool{false, true} {
		text := "selection input"
		tt := NewTester(func(c *Context) {
			input := TextInput(c, &text).Label("Input")
			if password {
				input.Password()
			}
		}, 320, 90)
		if err := tt.Click("Input"); err != nil {
			t.Fatal(err)
		}
		tt.Key(Ctrl, KeyA)
		tt.SetClipboard("original clipboard")
		tt.Key(Ctrl, KeyInsert)
		want := "selection input"
		if password {
			want = "original clipboard"
		}
		if got := tt.Clipboard(); got != want || text != "selection input" {
			t.Fatalf("password=%v: copied %q, input=%q", password, got, text)
		}
		tt.Key(0, KeyLeft)
		tt.SetClipboard("no selection")
		tt.Key(Ctrl, KeyInsert)
		if got := tt.Clipboard(); got != "no selection" {
			t.Fatalf("caret copied unselected text: %q", got)
		}
	}
}

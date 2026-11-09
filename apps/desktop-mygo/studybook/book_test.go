package studybook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func bookFixture(t *testing.T) (*Book, *testhost.Harness) {
	t.Helper()
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method != "learning.call" {
			return nil, fmt.Errorf("unexpected %s", method)
		}
		var input struct{ Method string }
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		switch input.Method {
		case "winone.overview":
			return map[string]any{"decks": []any{map[string]any{"id": "1", "name": "WinOne 生词本 · English vocabulary", "presetId": "1", "counts": map[string]int{"new": 4, "learning": 1, "review": 8, "total": 30}}}, "noteTypes": []any{map[string]string{"id": "1", "name": "WinOne Word"}}, "fsrs": true, "profiles": []string{"WinOne"}, "activeProfile": "WinOne"}, nil
		case "winone.browser":
			return map[string]any{"notes": []any{map[string]any{"id": "5", "noteType": map[string]string{"id": "1", "name": "WinOne Word"}, "fields": map[string]string{"Word": "hello", "Meaning": "你好；表示问候的表达"}, "tags": []string{"winone"}, "cardIds": []string{"6"}}}}, nil
		case "noteTypes.get":
			return map[string]any{"id": "1", "name": "WinOne Word", "fields": []any{map[string]string{"name": "Word"}, map[string]string{"name": "Meaning"}}}, nil
		case "editors.openNote", "editors.openAdd":
			return map[string]string{"id": "editor"}, nil
		case "editors.updateDraft":
			return nil, errors.New("fixture save failure")
		case "winone.editor.close":
			return map[string]bool{"closed": true}, nil
		default:
			return nil, fmt.Errorf("unexpected action %s", input.Method)
		}
	})
	b := New(h.Env)
	t.Cleanup(b.Close)
	return b, h
}

func TestNativeBookKeepsDraftAfterSaveFailure(t *testing.T) {
	b, h := bookFixture(t)
	tt := ui.NewTester(b.View, 800, 1200)
	h.Settle(tt)
	b.tab = 1
	b.search(false)
	h.Settle(tt)
	b.beginEdit(&b.notes[0])
	h.Settle(tt)
	b.edit.Fields["Meaning"] = "Edited meaning"
	b.tags = "edited"
	b.saveNote()
	h.Settle(tt)
	if b.edit == nil || b.edit.Fields["Meaning"] != "Edited meaning" || !b.Dirty() || !strings.Contains(b.err, "fixture save failure") {
		t.Fatalf("draft/error lost: %+v", b)
	}
}

func TestManagementPrefillKeepsSelectedCardIDs(t *testing.T) {
	b := &Book{form: toolForm{catalog: &toolCatalog{}}}
	b.openTool("cards.move", map[string]any{"ids": []string{"123", "456"}, "deck": map[string]string{"id": "1"}})
	ids, ok := b.form.value["ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "123" || ids[1] != "456" {
		t.Fatalf("selected IDs lost: %#v", b.form.value)
	}
}

func TestConnectDraftDiscardRestoresSavedSettings(t *testing.T) {
	b := &Book{}
	b.acceptConnect(connectSettings{Enabled: false, Port: 8765, Origins: []string{"https://example.invalid"}})
	b.connect.Enabled = true
	b.connectPort = "12345"
	b.connectOrigins = "https://changed.invalid"
	if !b.Dirty() {
		t.Fatal("connection draft is not protected")
	}
	b.Discard()
	if b.Dirty() || b.connect.Enabled || b.connectPort != "8765" || b.connectOrigins != "https://example.invalid" {
		t.Fatal("discard did not restore saved connection settings")
	}
}

func TestStudyTranslationsContainReadableText(t *testing.T) {
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		data, err := os.ReadFile(filepath.Join("..", "i18n", "study."+locale+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var table map[string]string
		if err = json.Unmarshal(data, &table); err != nil {
			t.Fatal(err)
		}
		for key, value := range table {
			if strings.Contains(value, "??") || strings.TrimSpace(value) == "" {
				t.Errorf("unreadable %s %s: %q", locale, key, value)
			}
		}
	}
}

func TestNativeBookLayouts(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, width := range []float32{420, 800, 1100} {
			for _, dark := range []bool{false, true} {
				b, h := bookFixture(t)
				view := func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), dark))
					ui.Column(c).FillWidth().Gap(16).Children(func() { b.View(c) })
				}
				tt := ui.NewTester(view, int(width), 1800)
				h.Settle(tt)
				for _, tab := range []int{0, 1, 3, 4} {
					b.tab = tab
					if tab == 1 {
						b.search(false)
					}
					h.Settle(tt)
					for _, label := range tt.Texts() {
						bounds, ok := tt.Find(label)
						if ok && (bounds.X < -.5 || bounds.X+bounds.W > width+.5) {
							t.Errorf("%s width=%v tab=%d overflow %q: %+v", locale, width, tab, label, bounds)
						}
					}
					if directory := os.Getenv("WINONE_SHOTS"); directory != "" {
						var buffer bytes.Buffer
						if err := png.Encode(&buffer, tt.Image()); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("study-%s-%v-%t-%d.png", locale, width, dark, tab)), buffer.Bytes(), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				for section := 1; section <= 4; section++ {
					b.tab = 4
					b.toolSection = section
					tt.Frame()
					for _, label := range tt.Texts() {
						bounds, ok := tt.Find(label)
						if ok && (bounds.X < -.5 || bounds.X+bounds.W > width+.5) {
							t.Errorf("%s width=%v tool=%d overflow %q: %+v", locale, width, section, label, bounds)
						}
					}
				}
			}
		}
	}
}

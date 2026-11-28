package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The correction must reach copy and subsequent actions without changing the
// conversation draft; cancelling an edit must leave the original untouched.
func TestOCREditSaveAndCancel(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Original, p.Result, p.Prompt = "型号 O01", "型号 O01", "未发送的问题"
	var copied string
	p.Actions.Copy = func(s string) { copied = s }
	tt := ui.NewTester(p.View, 520, 700)
	if err := tt.Click("编辑识别文字"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("识别文字编辑框"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("型号 001")
	if err := tt.Click("保存修改"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制原文"); err != nil {
		t.Fatal(err)
	}
	if copied != "型号 001" || p.Original != copied || p.Result != copied || p.Prompt != "未发送的问题" {
		t.Fatalf("correction lost: original=%q result=%q copied=%q draft=%q", p.Original, p.Result, copied, p.Prompt)
	}
	if err := tt.Click("编辑识别文字"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("识别文字编辑框"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("取消的修改")
	if err := tt.Click("取消修改"); err != nil {
		t.Fatal(err)
	}
	if p.Original != "型号 001" {
		t.Fatal("cancel changed the original")
	}
}

func TestOCREditKeepsEnterLocalAndCanEditAfterClearing(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "dark")
	p.Original = "原文"
	p.Result = p.Original
	var sent string
	p.Actions.Send = func(s string) { sent = s }
	tt := ui.NewTester(p.View, 420, 700)
	if err := tt.Click("编辑识别文字"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	if err := tt.Click("保存修改"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("编辑识别文字"); err != nil {
		t.Fatal(err)
	}
	tt.Type("第一行")
	tt.Key(0, ui.KeyEnter)
	tt.Type("第二行")
	if err := tt.Click("保存修改"); err != nil {
		t.Fatal(err)
	}
	if sent != "" || p.Original != "第一行\n第二行" {
		t.Fatalf("editor entered conversation: %q %q", sent, p.Original)
	}
}

func TestOCREditLayouts(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en-US"} {
		for _, theme := range []string{"light", "dark"} {
			for _, width := range []int{320, 700} {
				p := NewPanel("ocr", lang, theme)
				p.Original = "订单 001\nA multiline recognized fragment."
				p.Result = p.Original
				tt := ui.NewTester(p.View, width, 720)
				if err := tt.Click(p.T("编辑识别文字", "Edit recognized text")); err != nil {
					t.Fatal(err)
				}
				for _, label := range []string{p.T("识别文字编辑框", "Recognized text editor"), p.T("保存修改", "Save changes"), p.T("取消修改", "Cancel edit")} {
					bounds, ok := tt.Find(label)
					if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 || bounds.Y+bounds.H > 720 {
						t.Fatalf("%s/%s/%d: clipped %s %+v", lang, theme, width, label, bounds)
					}
				}
				if directory := os.Getenv("WINONE_SHOTS"); directory != "" {
					if err := os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(directory, fmt.Sprintf("ocr-edit-%s-%s-%d.png", lang, theme, width)))
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

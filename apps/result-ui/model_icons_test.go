package resultui

import (
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"winone/internal/ai"
)

func TestModelPickerPreservesTheReplyProviderIdentity(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("translate", "zh-CN", theme)
		p.Source, p.ActionLabel, p.Result = "selection", "解释", "这是由代理供应商提供的模型回答。"
		p.ResultDetails = Message{ProviderName: "代理供应商", ProviderIcon: "openrouter-color", Model: "gemini-2.5-pro"}
		p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "router", Name: "代理供应商", Icon: "openrouter-color", Models: []string{"gemini-2.5-pro", "claude-sonnet-4", "gpt-5"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "router", Model: "gemini-2.5-pro"}}}
		p.Messages = []Message{{Role: "user", Text: "继续解释"}, {Role: "assistant", Text: "这条回复继续显示供应商图标。", ProviderName: "代理供应商", ProviderIcon: "openrouter-color", Model: "gemini-2.5-pro"}}
		tt := ui.NewTester(p.View, 560, 700)
		if value := os.Getenv("WINONE_TEST_DPI_SCALE"); value != "" {
			scale, err := strconv.ParseFloat(value, 32)
			if err != nil {
				t.Fatal(err)
			}
			tt.SetScale(float32(scale))
		}
		save := func(stage string) {
			if directory := os.Getenv("WINONE_MODEL_ICON_LAYOUT_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(directory, theme+"-"+stage+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err = png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
		}
		save("reply")
		if err := tt.Click("切换供应商和模型"); err != nil {
			t.Fatal(err)
		}
		save("models")
		if err := tt.Click("claude-sonnet-4"); err != nil {
			t.Fatal(err)
		}
		if p.Selection == nil || p.Selection.Model != "claude-sonnet-4" {
			t.Fatal("model was not selected")
		}
		if p.ResultDetails.ProviderIcon != "openrouter-color" || p.Messages[1].ProviderIcon != "openrouter-color" || p.Messages[1].Model != "gemini-2.5-pro" {
			t.Fatal("model choice rewrote reply branding")
		}
	}
}

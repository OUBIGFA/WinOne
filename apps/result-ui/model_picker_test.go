package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

// Optional images come from the same native view used by capture and selection.
func TestModelPickerLayoutAndKeyboardAcrossWindows(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, source := range []string{"capture", "selection"} {
			for _, width := range []int{320, 420, 900} {
				p := pickerPanel()
				p.Theme, p.Source = theme, source
				p.Configuration.Providers[0].Name = strings.Repeat("团队服务", 12)
				p.Configuration.Providers[0].Models[0] = strings.Repeat("long-model-", 12)
				p.Configuration.Tasks["default"] = ai.Selection{Provider: "one", Model: p.Configuration.Providers[0].Models[0]}
				tt := ui.NewTester(p.View, width, 400)
				tt.SetScale(1.5)
				if err := tt.Click("切换供应商和模型"); err != nil {
					t.Fatal(err)
				}
				for _, label := range []string{"搜索供应商或模型", "可用模型", "使用任务默认"} {
					box, ok := tt.Find(label)
					if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > float32(width) || box.Y+box.H > 400 {
						t.Fatalf("%s at %d: popup outside window: %+v", label, width, box)
					}
				}
				if output := os.Getenv("WINONE_PICKER_LAYOUT_DIR"); output != "" {
					if err := os.MkdirAll(output, 0700); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(output, fmt.Sprintf("%s-%s-%d.png", theme, source, width))
					file, err := os.Create(path)
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatal(err, closeErr)
					}
				}
				// Arrows navigate only. Enter commits to the explicit provider/model.
				tt.Key(0, ui.KeyDown)
				tt.Key(0, ui.KeyDown)
				if p.Selection != nil {
					t.Fatal("arrow navigation committed a route")
				}
				tt.Key(0, ui.KeyEnter)
				if p.Selection == nil || p.Selection.Model != "claude-sonnet-4" {
					t.Fatalf("keyboard route: %+v", p.Selection)
				}
			}
		}
	}
}

func pickerPanel() *Panel {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original, p.Result = "Original text", "翻译结果"
	p.Configuration = ai.Configuration{Providers: []ai.Provider{
		{ID: "one", Name: "团队服务", Icon: "openrouter-color", Models: []string{"gemini-2.5-pro", "claude-sonnet-4"}},
		{ID: "two", Name: "个人服务", Icon: "deepseek-color", Models: []string{"gemini-2.5-pro", "deepseek-chat"}},
	}, Tasks: map[string]ai.Selection{"default": {Provider: "one", Model: "gemini-2.5-pro"}}}
	return p
}

func TestModelPickerTriggerFitsContent(t *testing.T) {
	p := pickerPanel()
	tt := ui.NewTester(p.View, 780, 600)
	trigger, _ := tt.Find("切换供应商和模型")
	if trigger.W > 220 {
		t.Fatalf("model trigger stretches across empty space: %+v", trigger)
	}
	for _, width := range []int{320, 420, 1200} {
		tt.SetSize(width, 600)
		trigger, _ = tt.Find("切换供应商和模型")
		send, _ := tt.Find("发送")
		if trigger.X+trigger.W > send.X || send.X+send.W > float32(width) {
			t.Fatal("model picker overlaps send")
		}
	}
}

func TestModelPickerSearchesProviderAndModelWithoutChangingRoute(t *testing.T) {
	p := pickerPanel()
	tt := ui.NewTester(p.View, 420, 520)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("团队服务") || !tt.HasText("个人服务") {
		t.Fatal("provider groups are not directly visible")
	}
	if err := tt.Click("搜索供应商或模型"); err != nil {
		t.Fatal(err)
	}
	tt.Type("个人 gemini")
	if p.Selection != nil {
		t.Fatal("filtering changed the route")
	}
	tt.Key(0, ui.KeyEnter)
	if p.Selection == nil || *p.Selection != (ai.Selection{Provider: "two", Model: "gemini-2.5-pro"}) || p.ModelPickerOpen {
		t.Fatal("provider/model search selected the wrong route")
	}
	if p.Configuration.Tasks["default"].Provider != "one" {
		t.Fatal("selection rewrote global settings")
	}
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("使用任务默认"); err != nil {
		t.Fatal(err)
	}
	if p.Selection != nil || p.ModelPickerOpen {
		t.Fatal("default route was not restored")
	}
}

func TestModelPickerEscapeAndEmptySearchKeepSelection(t *testing.T) {
	p := pickerPanel()
	tt := ui.NewTester(p.View, 320, 300)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索供应商或模型"); err != nil {
		t.Fatal(err)
	}
	tt.Type("no-such-model")
	if !tt.HasText("没有匹配的模型") {
		t.Fatal("empty search has no feedback")
	}
	tt.Key(0, ui.KeyEnter)
	if p.Selection != nil {
		t.Fatal("empty search changed selection")
	}
	tt.Key(0, ui.KeyEscape)
	if p.ModelPickerOpen {
		t.Fatal("Escape did not close picker")
	}
}

func TestProviderGroupsCollapseIndependentlyAndSearchRestoresThem(t *testing.T) {
	p := pickerPanel()
	tt := ui.NewTester(p.View, 420, 650)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("供应商：团队服务"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("claude-sonnet-4") || !tt.HasText("deepseek-chat") || p.Selection != nil || !p.ModelPickerOpen {
		t.Fatal("collapsing one provider changed the route or another provider")
	}
	if err := tt.Click("搜索供应商或模型"); err != nil {
		t.Fatal(err)
	}
	tt.Type("claude")
	if !tt.HasText("claude-sonnet-4") {
		t.Fatal("search did not reveal a model in a collapsed provider")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	if tt.HasText("claude-sonnet-4") || !tt.HasText("deepseek-chat") {
		t.Fatal("clearing search did not restore collapsed groups")
	}
	if err := tt.Click("供应商：团队服务"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("claude-sonnet-4") {
		t.Fatal("provider did not expand")
	}
	// Space toggles the focused disclosure; it must not select a model.
	tt.Key(0, ui.KeySpace)
	if tt.HasText("claude-sonnet-4") || p.Selection != nil {
		t.Fatal("keyboard collapse changed selection")
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("claude-sonnet-4") {
		t.Fatal("reopening forgot collapsed provider")
	}
	if err := tt.Click("供应商：个人服务"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("团队服务") || !tt.HasText("个人服务") || tt.HasText("deepseek-chat") || tt.HasText("没有匹配的模型") {
		t.Fatal("all collapsed groups lost their expandable headings")
	}
	if err := tt.Click("搜索供应商或模型"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	if p.Selection != nil || !p.ModelPickerOpen {
		t.Fatal("Enter selected a hidden model")
	}
}

package resultui

import (
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestFreeModelBadgeFitsNativePicker(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		panel := pickerPanel()
		panel.Theme = theme
		panel.Configuration.Providers[0].ModelFree = map[string]bool{"claude-sonnet-4": true}
		tt := ui.NewTester(panel.View, 420, 400)
		if err := tt.Click("切换供应商和模型"); err != nil {
			t.Fatal(err)
		}
		badge, ok := tt.Find("免费模型")
		row, found := tt.Find("claude-sonnet-4")
		if !ok || !found || badge.X < row.X || badge.X+badge.W > row.X+row.W || badge.Y < row.Y || badge.Y+badge.H > row.Y+row.H {
			t.Fatal("free badge escaped model row", badge, row)
		}
		if tt.HasText("claude-sonnet-4 (免费)") {
			t.Fatal("badge was appended to model name")
		}
		if directory := os.Getenv("WINONE_PICKER_LAYOUT_DIR"); directory != "" {
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(directory, "free-"+theme+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestModelPickerSearchesLocalizedFreeMetadataAndKeepsIDs(t *testing.T) {
	provider := ai.Provider{ID: "service", Name: "Service", Models: []string{"opaque", "freedom"}, ModelNames: map[string]string{"opaque": "Model A"}, ModelFree: map[string]bool{"opaque": true}}
	panel := &Panel{Language: "zh-CN", Configuration: ai.Configuration{Providers: []ai.Provider{provider}}}
	rows := panel.modelPickerRows("免费", nil)
	if len(rows) != 2 || rows[1].model != "opaque" {
		t.Fatal("free search lost the original route", rows)
	}
	if panel.modelLabel(provider, "freedom") != "freedom" {
		t.Fatal("non-free name was marked")
	}
	panel.Language = "en-US"
	if label := panel.modelLabel(provider, "opaque"); label != "Model A" {
		t.Fatal(label)
	}
}

func TestAllModelBadgesFitNativeTrigger(t *testing.T) {
	for _, width := range []int{320, 540} {
		panel := pickerPanel()
		provider := &panel.Configuration.Providers[0]
		id := provider.Models[0]
		provider.ModelFree = map[string]bool{id: true}
		provider.ModelMetadata = map[string]ai.ModelMetadata{id: {Rate: 0.5, RateWas: 2, ListedContextWindow: 1000000}}
		panel.Selection = &ai.Selection{Provider: provider.ID, Model: id}
		panel.TranslationSelection = &ai.Selection{Provider: "two", Model: "deepseek-chat"}
		tt := ui.NewTester(panel.View, width, 500)
		trigger, found := tt.Find("切换供应商和模型")
		if !found {
			t.Fatal("model trigger missing")
		}
		for _, label := range []string{"免费模型", "额度倍率", "百万级上下文"} {
			badge, ok := tt.Find(label)
			if !ok || badge.W <= 0 || badge.X < trigger.X || badge.X+badge.W > trigger.X+trigger.W+1 || badge.Y < trigger.Y || badge.Y+badge.H > trigger.Y+trigger.H+1 {
				t.Fatalf("%s is missing or outside the model trigger at %d DIP: %+v, trigger %+v", label, width, badge, trigger)
			}
		}
		for _, label := range []string{"免费", "2×", "0.5×", "1M"} {
			if !tt.HasText(label) {
				t.Fatalf("badge text missing: %s", label)
			}
		}
	}
}

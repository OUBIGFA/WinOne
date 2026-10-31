package widgets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

func aiPickTestProviders(t *testing.T) []AIProvider {
	t.Helper()
	var providers []AIProvider
	err := json.Unmarshal([]byte(`[
	 {"id":"work","name":"工作服务","icon":"openai","models":["openai/gpt-5","azure/gpt-5","free-model","deep"],
	  "availableModels":["openai/gpt-5","azure/gpt-5","free-model","deep","off"],
	  "modelNames":{"openai/gpt-5":"GPT-5","azure/gpt-5":"GPT-5","deep":"Deep (免费)"},
	  "modelMetadata":{"openai/gpt-5":{"rate":0.5,"rateWas":1},"deep":{"contextWindow":1048576},"free-model":{"listedContextWindow":2000000,"rate":2}}},
	 {"id":"empty","name":"空服务","models":[]}
	]`), &providers)
	if err != nil {
		t.Fatal(err)
	}
	return providers
}

func TestAIModelLabelsTellSharedNamesApartAndDropFreeSuffixes(t *testing.T) {
	providers := aiPickTestProviders(t)
	work := &providers[0]
	for id, want := range map[string]string{
		"openai/gpt-5": "GPT-5 · openai", "azure/gpt-5": "GPT-5 · azure", "deep": "Deep", "free-model": "free-model", "unknown": "unknown",
	} {
		if got := ModelLabel(work, id); got != want {
			t.Errorf("ModelLabel(%q) = %q, want %q", id, got, want)
		}
	}
	if !IsFreeModel(work, "free-model") || !IsFreeModel(work, "deep") || IsFreeModel(work, "openai/gpt-5") {
		t.Error("free models are told by their IDs and names")
	}
	freedom := &AIProvider{ModelNames: map[string]string{"freedom": "Freedom"}}
	if IsFreeModel(freedom, "freedom") || !IsFreeModel(&AIProvider{ModelFree: map[string]bool{"x": true}}, "x") {
		t.Error("Freedom is not free; vendor metadata is")
	}
	if aiPickTimes(0.5) != "0.5×" || aiPickTimes(1) != "1×" || aiPickTimes(0.004) != "<0.01×" || aiPickTimes(1.236) != "1.24×" {
		t.Error("rates show two decimals at most")
	}
	if tells := aiPickTells([]string{"a/x/b", "a/y/b"}, func(string) string { return "Same" }); tells["a/x/b"] != "x" || tells["a/y/b"] != "y" {
		t.Errorf("tells %v", tells)
	}
}

// aiPickTestView is a task's picker in a view, as pages use it.
type aiPickTestView struct {
	h         *testhost.Harness
	providers []AIProvider
	value     *AISelection
	inherit   bool
	changes   int
}

func (v *aiPickTestView) view(c *ui.Context) {
	inherit := ""
	if v.inherit {
		inherit = i18n.T("ai.followDefaultService")
	}
	ui.Column(c).Padding(16).Children(func() {
		if next, changed := AIModelSelect(c, AIModelSelectOptions{Env: v.h.Env, Providers: v.providers, Value: v.value, Label: "任务", InheritLabel: inherit}); changed {
			v.value = next
			v.changes++
		}
	})
}

func aiPickTestSetup(t *testing.T, value *AISelection, inherit bool) (*aiPickTestView, *ui.Tester) {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
	v := &aiPickTestView{h: h, providers: aiPickTestProviders(t), value: value, inherit: inherit}
	tt := ui.NewTester(v.view, 900, 700)
	return v, tt
}

func aiPickTestClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
}

func TestAIModelSelectFollowsTheDefaultAndChoosesTheFirstModel(t *testing.T) {
	v, tt := aiPickTestSetup(t, nil, true)
	if !tt.HasText("跟随 AI 服务默认设置") || !tt.HasText("使用默认模型") || tt.HasText("未配置") {
		t.Fatalf("texts %q", tt.Texts())
	}
	aiPickTestClick(t, tt, "任务 · 供应商")
	aiPickTestClick(t, tt, "工作服务")
	if v.value == nil || v.value.Provider != "work" || v.value.Model != "openai/gpt-5" {
		t.Fatalf("value %+v", v.value)
	}
	aiPickTestClick(t, tt, "任务 · 模型名称")
	for _, badge := range []string{"免费", "0.5×", "1×", "1.0M", "2M"} {
		if !tt.HasText(badge) {
			t.Errorf("badge %q missing: %q", badge, tt.Texts())
		}
	}
	if tt.HasText("off") {
		t.Error("models not enabled are not offered")
	}
	aiPickTestClick(t, tt, "Deep")
	if v.value.Model != "deep" {
		t.Fatalf("value %+v", v.value)
	}
	aiPickTestClick(t, tt, "任务 · 供应商")
	aiPickTestClick(t, tt, "跟随 AI 服务默认设置")
	if v.value != nil || v.changes != 3 {
		t.Fatalf("following the default is no selection: %+v after %d changes", v.value, v.changes)
	}
}

func TestAIModelSelectKeepsUnavailableChoicesVisible(t *testing.T) {
	v, tt := aiPickTestSetup(t, &AISelection{Provider: "gone", Model: "old"}, true)
	if !tt.HasText("gone") || !tt.HasText("old 未启用或已移除，请重新选择。") {
		t.Fatalf("a removed provider stays shown: %q", tt.Texts())
	}
	v.value = &AISelection{Provider: "work", Model: "off"}
	tt.Frame()
	if !tt.HasText("off 未启用或已移除，请重新选择。") {
		t.Fatalf("a disabled model is reported: %q", tt.Texts())
	}
	v.value = &AISelection{}
	tt.Frame()
	if !tt.HasText("未配置") {
		t.Fatalf("a selection without a provider shows as not configured: %q", tt.Texts())
	}
	v.value = &AISelection{Provider: "empty"}
	tt.Frame()
	if !tt.HasText("请先在 AI 服务中勾选模型") || !strings.Contains(strings.Join(tt.Texts(), "|"), "空服务") {
		t.Fatalf("a provider without enabled models: %q", tt.Texts())
	}
}

func TestAIModelSelectWithoutInheritOffersChoosingAProvider(t *testing.T) {
	v, tt := aiPickTestSetup(t, nil, false)
	if !tt.HasText("选择供应商") || !tt.HasText("请先选择供应商") || tt.HasText("跟随 AI 服务默认设置") {
		t.Fatalf("texts %q", tt.Texts())
	}
	aiPickTestClick(t, tt, "任务 · 供应商")
	aiPickTestClick(t, tt, "空服务")
	if v.value == nil || v.value.Provider != "empty" || v.value.Model != "" {
		t.Fatalf("value %+v", v.value)
	}
}

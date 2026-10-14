package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestResultIdentityFollowsSelectionAndCapturePresentation(t *testing.T) {
	for _, mode := range []string{"ocr", "translate"} {
		for _, source := range []string{"capture", "selection"} {
			for _, streaming := range []bool{false, true} {
				t.Run(source+"-"+mode+map[bool]string{false: "-complete", true: "-streaming"}[streaming], func(t *testing.T) {
					p := NewPanel(mode, "zh-CN", "light")
					p.Source = source
					p.Original = "原文"
					identity := Message{ProviderName: "实际供应商", Model: "实际模型", Text: "结果内容"}
					if streaming {
						p.Busy = true
						p.ResultPending = &identity
					} else {
						p.Result = "结果内容"
						p.ResultDetails = identity
					}
					tt := ui.NewTester(p.View, 620, 660)
					if _, ok := tt.Find("实际供应商"); ok != (source == "selection" || mode == "translate") {
						t.Fatal("provider identity does not match the result presentation")
					}
					if _, ok := tt.Find("实际模型"); ok != (source == "selection" || mode == "translate") {
						t.Fatal("model identity does not match the result presentation")
					}
				})
			}
		}
	}
}

package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/internal/resultwindow"
)

func TestQuickAssistantStartsWithChatAndSharedActions(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Mode: "chat", Source: "quick-assistant", Language: "zh-CN", Theme: "light"})
	if c.Panel.Pinned {
		t.Fatal("quick assistant must not start pinned by default")
	}
	c.Panel.Busy = false
	tt := ui.NewTester(c.Panel.View, 520, 600)
	if _, ok := tt.Find("输入消息"); !ok {
		t.Fatal("assistant must open as a chat composer")
	}
	for _, label := range []string{"翻译", "解释", "总结", "润色", "代码"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing shared action %s", label)
		}
	}
	if c.Panel.Result != "" || len(c.seed()) != 0 {
		t.Fatal("empty chat must not contain capture results")
	}
}

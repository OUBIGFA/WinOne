package resultui

import (
 "math"
 "strings"
 "testing"
 "github.com/egoist/mygo/ui"
)

func TestLongThinkingCollapseStaysFixedAndShortThinkingDoesNotAddIt(t *testing.T){
 panel:=NewPanel("chat","zh-CN","light");panel.Source="quick-assistant"
 panel.Messages=[]Message{{Role:"assistant",Text:"最终回答",Reasoning:"短思考",ReasoningOpen:true}}
 view:=ui.NewTester(panel.View,520,700);view.SetPreferences(ui.Preferences{ReduceMotion:true,TextScale:1});view.Frame()
 if _,ok:=view.Find("收起思考过程");ok{t.Fatal("short thinking added a persistent action")}
 panel.Messages[0].Reasoning=strings.Repeat("这是一段需要滚动阅读的思考内容。\n",80);view.Frame();view.Frame()
 button,ok:=view.Find("收起思考过程");if !ok{t.Fatal("long thinking has no persistent collapse action")}
 viewport,ok:=view.Find("思考内容");if !ok{t.Fatal("thinking viewport missing")}
 if viewport.H>281{t.Fatal("thinking exceeded the shared 40vh/320 DIP limit",viewport)}
 view.Scroll(viewport.X+20,viewport.Y+20,0,500);view.Frame()
 after,ok:=view.Find("收起思考过程");if !ok||math.Abs(float64(after.Y-button.Y))>.5{t.Fatal("collapse action moved with thinking scroll",button,after)}
 if panel.Messages[0].thinkingScroll.Y==0{t.Fatal("the long thinking content did not scroll")}
 if err:=view.Click("收起思考过程");err!=nil{t.Fatal(err)};view.Frame()
 if panel.Messages[0].ReasoningOpen{t.Fatal("persistent action did not collapse thinking")}
}

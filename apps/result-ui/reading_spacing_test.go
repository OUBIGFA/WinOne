package resultui

import (
 "fmt"
 "math"
 "testing"
 "github.com/egoist/mygo/ui"
)

// Measure rendered content boxes rather than adding style constants together.
// This catches extra parent gaps and margins in the actual result composition.
func TestThinkingAnswerHasOneSpacingBoundary(t *testing.T){
 for _,source:=range []string{"quick-assistant","selection","capture"}{
  for _,pending:=range []bool{false,true}{
   for _,expanded:=range []bool{false,true}{
    for _,width:=range []int{360,720}{
     for _,heading:=range []bool{false,true}{
     t.Run(fmt.Sprintf("%s/pending=%t/expanded=%t/width=%d/heading=%t",source,pending,expanded,width,heading),func(t *testing.T){
      panel:=NewPanel("chat","zh-CN","light");panel.Source=source
      message:=Message{Role:"assistant",Reasoning:"思考内容在这里结束。",Text:"正文从这里开始。\n\n第二段正文。",ReasoningOpen:expanded}
      if heading{message.Text="## 正文从这里开始。\n\n第二段正文。"}
      if pending{message.State="running";panel.Pending=&message;panel.Busy=true}else{panel.Messages=[]Message{message}}
      view:=ui.NewTester(panel.View,width,900);view.SetPreferences(ui.Preferences{ReduceMotion:true,TextScale:1})
      upperLabel:="思考过程";if expanded{upperLabel=message.Reasoning}
      upper,ok:=view.Find(upperLabel);if !ok{t.Fatal("thinking boundary not rendered",view.Texts())}
      lower,ok:=view.Find("正文从这里开始。");if !ok{t.Fatal("answer not rendered",view.Texts())}
      gap:=lower.Y-(upper.Y+upper.H)
      if math.Abs(float64(gap-16))>0.6{t.Fatalf("thinking-to-answer gap = %.2f DIP, want 16; upper=%+v lower=%+v",gap,upper,lower)}
     })
     }
    }
   }
  }
 }
}

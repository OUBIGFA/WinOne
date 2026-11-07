package pluginhost

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "strings"
 "testing"
 "github.com/egoist/mygo/ui"
 "winone/desktopmygo/kit"
 "winone/desktopmygo/testhost"
)

func TestNativeCardFormsPreserveTypingAndSubmitTheirDeclaredValues(t *testing.T){
 var submitted map[string]any
 harn:=testhost.New(t,func(method string,raw json.RawMessage)(any,error){if method=="chat.ghosts.card-action"{_ =json.Unmarshal(raw,&submitted);return map[string]bool{"ok":true},nil};return nil,nil})
 host:=For(harn.Env);defer host.Close()
 raw:=json.RawMessage(`{"callId":"card","viewHtml":"<div><button data-ghost-action=\"go\">Submit</button></div>","html":"<div><button data-ghost-action=\"go\">Submit</button></div>","fields":[{"name":"name","label":"Name","value":"Initial"}]}`)
 host.cards.update(raw)
 item:=host.cards.byCall["card"]
 item.values["name"]="Typed locally"
 host.cards.update(json.RawMessage(`{"callId":"card","viewHtml":"<div><button data-ghost-action=\"go\">Submit</button></div>","html":"<div><button data-ghost-action=\"go\">Submit</button></div>","fields":[{"name":"name","label":"Name","value":"Updated by plugin"}]}`))
 if item.values["name"]!="Typed locally"{t.Fatal("card broadcast replaced an unsaved value")}
 tt:=ui.NewTester(func(c *ui.Context){c.SetTheme(kit.Theme(c.Preferences(),false));host.Card(c,"card")},700,500)
 harn.Settle(tt)
 if err:=tt.Click("Submit");err!=nil{t.Fatal(err)}
 harn.Settle(tt)
 fields,_:=submitted["formValues"].(map[string]any)
 if submitted["action"]!="go"||fields["name"]!="Typed locally"{t.Fatalf("submitted values: %#v",submitted)}
}

func TestDerivedCardsDisplayBeneathTheirOwnParentAndRepliesDoNotUseTheLatestMessage(t *testing.T){
 harn:=testhost.New(t,func(string,json.RawMessage)(any,error){return nil,nil})
 host:=For(harn.Env);defer host.Close()
 host.cards.update(json.RawMessage(`{"callId":"root","html":"<div>Parent</div>","children":[{"callId":"root::sp1","rootCallId":"root","html":"<div>Derived result</div>"}]}`))
 tt:=ui.NewTester(func(c *ui.Context){c.SetTheme(kit.Theme(c.Preferences(),false));host.Card(c,"root")},700,500)
 harn.Settle(tt)
 if !tt.HasText("Parent")||!tt.HasText("Derived result"){t.Fatalf("derived content omitted: %v",tt.Texts())}
 hash:=sha256.Sum256([]byte("Original"))
 host.storeRewrite(replyOverride{SessionID:"session",MessageID:"a1",SourceSHA256:hex.EncodeToString(hash[:]),Action:"rewrite",Text:"Shown"})
 if host.Reply("session","a1","Original")==nil||host.Reply("session","a2","Original")!=nil||host.Reply("session","a1","Different")!=nil{t.Fatal("rewrite applied to a different message or source")}
 host.storeRewrite(replyOverride{SessionID:"session",MessageID:"a1",SourceSHA256:hex.EncodeToString(hash[:]),Action:"render",HTML:"<div>Rendered</div>"})
 tt=ui.NewTester(func(c *ui.Context){c.SetTheme(kit.Theme(c.Preferences(),false));host.RenderOverride(c,"session","a1","Original")},700,500)
 harn.Settle(tt)
 if err:=tt.Click(text("showOriginal"));err!=nil{t.Fatal(err)}
 harn.Settle(tt)
 if !strings.Contains(strings.Join(tt.Texts()," "),"Original"){t.Fatal("rendered override cannot expose the model's original answer")}
}

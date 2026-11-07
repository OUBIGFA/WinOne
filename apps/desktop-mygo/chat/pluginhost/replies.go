package pluginhost

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "github.com/egoist/mygo/ui"
 "winone/desktopmygo/kit"
)

type replyOverride struct {
 SessionID string `json:"sessionId"`
 MessageID string `json:"messageId"`
 SourceSHA256 string `json:"sourceSHA256"`
 Action string `json:"action"`
 Text string `json:"text"`
 HTML string `json:"html"`
 GhostName string `json:"ghostName"`
 showOriginal bool
}
func(h *Host)storeRewrite(value replyOverride){
 if value.SessionID==""||value.MessageID==""||len(value.SourceSHA256)!=64{return}
 if h.replies==nil{h.replies=map[string]map[string]*replyOverride{}}
 if h.replies[value.SessionID]==nil{h.replies[value.SessionID]=map[string]*replyOverride{}}
 h.replies[value.SessionID][value.MessageID]=&value
}
func(h *Host)LoadReplies(session string){
 if session==""||h.closed||h.repliesAsked[session]{return}
 if h.repliesAsked==nil{h.repliesAsked=map[string]bool{}}
 h.repliesAsked[session]=true
 h.call("reply-overrides",map[string]string{"sessionId":session},func(raw json.RawMessage,err error){
  if err!=nil{return}
  var rows []replyOverride
  if json.Unmarshal(raw,&rows)==nil{for _,row:=range rows{h.storeRewrite(row)}}
 })
}
func(h *Host)Reply(session,id,original string)*replyOverride{
 value:=h.replies[session][id]
 if value==nil{return nil}
 hash:=sha256.Sum256([]byte(original))
 if hex.EncodeToString(hash[:])!=value.SourceSHA256{return nil}
 return value
}
// RenderOverride presents the sanitized post-hook card without hiding what the
// model actually produced. This is distinct from an in-flight tool card.
func(h *Host)RenderOverride(c *ui.Context,session,id,original string)bool{
 value:=h.Reply(session,id,original)
 if value==nil||value.Action!="render"||value.HTML==""{return false}
 key:="reply:"+session+":"+id
 item:=h.cards.byCall[key]
 if item==nil{item=&card{CallID:key};h.cards.byCall[key]=item}
 item.HTML=value.HTML
 root:=h.cards.parsed(item)
 ui.Column(c.Key(key)).FillWidth().Gap(8).Children(func(){
  if body:=findBody(root);body!=nil{h.cardChildren(c,item,body,key)}
  label:=text("showOriginal");if value.showOriginal{label=text("hideOriginal")}
  if kit.GhostButton(c,label,nil,false).Clicked(){value.showOriginal=!value.showOriginal}
  if value.showOriginal{ui.Text(c,original).Selectable().MinWidth(0)}
 })
 return true
}

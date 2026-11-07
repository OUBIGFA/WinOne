package automations

import (
 "encoding/json"
 "fmt"
)

// OpenPluginDraft consumes Cindy's already-sanitized schedule draft. It never
// calls create/update. An existing form wins, including a pristine open form,
// so a plugin cannot replace what the owner is currently reviewing.
func(p *Page)OpenPluginDraft(raw json.RawMessage)bool{
 if p.closed||p.draft!=nil||p.pluginDraft!=nil||p.Busy(){return false}
 var input struct{RequestID,Name,Prompt string;IntervalMs *int64}
 if json.Unmarshal(raw,&input)!=nil||input.RequestID==""||input.Name==""||input.Prompt==""{return false}
 if p.pluginDraftIDs[input.RequestID]{return false}
 if !p.loaded {p.pluginDraft=append(json.RawMessage(nil),raw...);p.SetVisible(true);return true}
 if p.pluginDraftIDs==nil{p.pluginDraftIDs=map[string]bool{}}
 p.pluginDraftIDs[input.RequestID]=true
 p.draft=newDraft(nil,p.runtime.Timezone)
 p.draft.Value.Name,p.draft.Value.Prompt=input.Name,input.Prompt
 if input.IntervalMs!=nil{
  interval:=max(*input.IntervalMs,30*60*1000)
  p.draft.Value.IntervalMs=&interval
  p.draft.timing,p.draft.intervalUnit,p.draft.interval="interval","minutes",fmt.Sprintf("%g",float64(interval)/60000)
 }
 p.loadFormResources()
 return true
}

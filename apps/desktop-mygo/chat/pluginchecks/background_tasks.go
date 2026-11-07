package pluginchecks

import (
 "encoding/json"
 "github.com/egoist/mygo/ui"
 "winone/desktopmygo/core"
 "winone/desktopmygo/kit"
 "winone/desktopmygo/kit/icons"
)

type backgroundTask struct {TaskID string `json:"taskId"`;Title string `json:"title"`;Status string `json:"status"`}
func(m *Manager)loadBackgroundTasks(id string){
 m.backgroundTasks=nil;m.backgroundLoading=true
 core.Go(m.env,func()([]backgroundTask,error){
  var rows []backgroundTask;err:=m.env.Host.CallInto("chat.ghosts.background-tasks",map[string]string{"pluginId":id},&rows);return rows,err
 },func(rows []backgroundTask,err error){
  if !m.open||m.configuration==nil||m.configuration.PluginID!=id{return};m.backgroundLoading=false
  if err!=nil{m.error=err.Error();return};m.backgroundTasks=rows
 })
}
func(m *Manager)backgroundTaskFields(c *ui.Context){
 if len(m.backgroundTasks)==0{return}
 kit.Strong(c,text("backgroundTasks"))
 kit.Help(c,text("backgroundTasksHint"))
 for _,task:=range m.backgroundTasks{
  row:=task
  ui.Row(c.Key(row.TaskID)).FillWidth().Gap(8).Children(func(){
   kit.Caption(c,row.Title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(row.Title)
   if kit.Button(c,text("revealTask"),icons.ArrowUpRight,false).Disabled(m.busy).Clicked(){
    id:=m.configuration.PluginID
    m.env.RequestChange(func(){m.call("reveal-background-task",map[string]string{"pluginId":id,"taskId":row.TaskID},func(json.RawMessage){m.loadBackgroundTasks(id)})})
   }
  })
 }
}

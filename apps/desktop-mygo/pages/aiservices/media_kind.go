package aiservices

import (
 "github.com/egoist/mygo/ui"
 "winone/desktopmygo/i18n"
 "winone/desktopmygo/kit"
 "winone/desktopmygo/widgets"
)

// Magpie's explicit image/video kind setting is preserved for providers whose
// catalog gives no type. Empty removes the override and follows the vendor.
func(a *advancedPanel)mediaKindFields(c *ui.Context,locked bool){
 if a.p.draft==nil{return}
 ui.Field(c,i18n.T("ai.mediaKindModel"),func(){
  next,changed:=widgets.ModelField(c,widgets.ModelFieldOptions{ModelMenuOptions:widgets.ModelMenuOptions{Env:a.p.env,Groups:widgets.ProviderGroups(pickers(a.p.draft.Value.Providers)),Selected:a.mediaKindModel,Label:i18n.T("ai.mediaKindModel")},Placeholder:i18n.T("ai.mediaKindChoose"),Disabled:locked})
  if changed{a.mediaKindModel=next}
 })
 selected,ok:=widgets.ParseSelectionValue(a.mediaKindModel)
 if !ok{return}
 key:=selected.Provider+"/"+selected.Model
 settings:=a.settings();kind:=settings.ModelKinds[key]
 ui.Field(c,i18n.T("ai.mediaKind"),func(){
  if changed,_:=kit.Select(c,&kind,[]kit.SelectOption{{Value:"",Label:i18n.T("ai.mediaKindDefault")},{Value:"image",Label:i18n.T("ai.mediaKindImage")},{Value:"video",Label:i18n.T("ai.mediaKindVideo")}});changed{
   if kind==""{delete(settings.ModelKinds,key)}else{if settings.ModelKinds==nil{settings.ModelKinds=map[string]string{}};settings.ModelKinds[key]=kind}
  }
 })
 kit.Help(c,i18n.T("ai.mediaKindHint"))
}

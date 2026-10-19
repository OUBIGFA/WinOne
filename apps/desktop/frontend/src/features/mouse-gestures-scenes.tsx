import {usePathDrop} from "../ui/path-drop";
import {useEffect,useState} from "react";
import {AppWindow,CodeXml,FolderOpen,Globe,Keyboard,Monitor,Mouse,Music,PanelsTopLeft,Plus,Settings,Terminal,Trash2} from "lucide-react";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Textarea} from "../components/ui/textarea";
import {Checkbox} from "../components/ui/checkbox";
import {Field,FieldLabel} from "../components/ui/field";
import {Dialog,DialogContent,DialogDescription,DialogHeader,DialogTitle} from "../components/ui/dialog";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {SettingsOptions} from "../components/settings-layout";
import {StrokeGestureThumbnail} from "./mouse-gestures-canvas";
import {escapedProcessPattern,StrokePathPicker} from "./mouse-gestures-controls";
import {addStrokeSceneAction,orderedStrokeScopes,removeStrokeSceneAction,strokeSceneActions,updateStrokeSceneAction} from "./mouse-gestures-scenes-model";
import {strokeId,type StrokeAction,type StrokePreferences,type StrokeScope} from "./mouse-gestures-types";
import "./mouse-gestures.css";

type Selection={packageId:string;actionId:string};
type Assignment={source:string;name:string;gesture:string;code:string};

// Built-in scopes keep task-specific symbols; custom application rules retain
// the generic application icon unless their name/path identifies a task.
function sceneIcon(scope:StrokeScope){
 const builtIn:Record<string,typeof AppWindow>={"explorer-folders":FolderOpen,global:Monitor,media:Music,windows:AppWindow,tabs:PanelsTopLeft,browser:Globe,applications:AppWindow,keyboard:Keyboard};
 if(builtIn[scope.id])return builtIn[scope.id];
 if(scope.kind==="explorer-folders")return FolderOpen;
 const identity=[scope.id,scope.name,scope.processPath,scope.pattern].join(" ");
 if(/browser|chrome|firefox|msedge|catsxp|浏览器/i.test(identity))return Globe;
 if(/terminal|powershell|cmd\.exe|终端/i.test(identity))return Terminal;
 if(/code|editor|notepad|编辑器/i.test(identity))return CodeXml;
 if(/music|media|player|音乐|播放器/i.test(identity))return Music;
 return AppWindow;
}

// WGestures' app -> gesture -> command flow and Stroke's complete package
// semantics are assembled from WinOne's existing Base UI controls. The model
// keeps scene edits isolated without changing the persisted Stroke format.
export function StrokeScenes({preferences,scopeId,onScopeChange,disabled,onChange,onInteractionChange,onManageGesture}:{preferences:StrokePreferences;scopeId:string;onScopeChange:(id:string)=>void;disabled:boolean;onChange:(patch:Partial<StrokePreferences>)=>void;onInteractionChange:(active:boolean)=>void;onManageGesture:(id:string)=>void}){
 const {t}=useTranslation();
 const [settingsOpen,setSettingsOpen]=useState(false),[editor,setEditor]=useState<Selection>(),[assignment,setAssignment]=useState<Assignment>(),[error,setError]=useState("");
 const scopes=orderedStrokeScopes(preferences.scopes),scope=scopes.find(item=>item.id===scopeId)??scopes[0];
 const entries=scope?strokeSceneActions(preferences,scope):[];
 const editingPackage=preferences.actionPackages.find(pack=>pack.id===editor?.packageId),editingAction=editingPackage?.actions.find(action=>action.id===editor?.actionId);
 const gestures=[...preferences.gestures.map(gesture=>({value:gesture.id,label:gesture.name})),...Array.from({length:7},(_,index)=>({value:"#"+index,label:t("stroke.special"+index)}))];
 const sources=preferences.actionPackages.flatMap(pack=>pack.actions.map(action=>({value:pack.id+"/"+action.id,label:pack.name+" · "+action.name,action})));
 useEffect(()=>{onInteractionChange(settingsOpen||!!editor||!!assignment);return()=>onInteractionChange(false);},[settingsOpen,editor,assignment,onInteractionChange]);
 const processDrop=usePathDrop({kind:"file",disabled,onPaths:paths=>changeScope({processPath:paths[0]}),onError:setError});
 const patternDrop=usePathDrop({kind:"file",multiple:true,disabled,onPaths:paths=>changeScope({pattern:[scope?.pattern,...paths.map(escapedProcessPattern)].filter(Boolean).join("\n")}),onError:setError});
 function changeScene(id:string,patch:Partial<StrokeScope>){onChange({scopes:preferences.scopes.map(item=>item.id===id?{...item,...patch}:item)});}
 function changeScope(patch:Partial<StrokeScope>){if(scope)changeScene(scope.id,patch);}
 function updateAction(packageId:string,actionId:string,patch:Partial<StrokeAction>){
  if(!scope)return;
  try{const next=updateStrokeSceneAction(preferences,scope.id,packageId,actionId,patch);onChange(next.preferences);if(editor)setEditor({...editor,packageId:next.packageId});setError("");}catch(error){setError(t("stroke."+(error instanceof Error?error.message:"missingSceneAction")));}
 }
 function addScope(){const id=strokeId();onChange({scopes:[...preferences.scopes,{id,name:t("stroke.newScope"),kind:"application",enabled:false,pattern:"",packages:[]}]});onScopeChange(id);setSettingsOpen(true);setError("");}
 function openAssignment(){const source=sources[0];setAssignment({source:source?.value??"custom",name:source?.action.name??t("stroke.newAction"),gesture:source?.action.gesture??preferences.gestures[0]?.id??"#1",code:source?.action.code??""});setError("");}
 function addAssignment(){
  if(!scope||!assignment)return;
  try{const next=addStrokeSceneAction(preferences,scope.id,{name:assignment.name,gesture:assignment.gesture,code:assignment.code,enabled:true});onChange(next.preferences);setAssignment(undefined);setError("");}catch(error){setError(t("stroke."+(error instanceof Error?error.message:"missingSceneAction")));}
 }
 function removeAction(){
  if(!scope||!editor)return;
  try{onChange(removeStrokeSceneAction(preferences,scope.id,editor.packageId,editor.actionId).preferences);setEditor(undefined);setError("");}catch(error){setError(t("stroke."+(error instanceof Error?error.message:"missingSceneAction")));}
 }
 return <div className="stroke-scenes settings-stack">
  <div className="stroke-scene-toolbar"><Button icon={Plus} disabled={disabled||scopes.length>=64} onClick={addScope}>{t("stroke.addScope")}</Button></div>
  {error&&!settingsOpen&&!editor&&!assignment&&<Notice>{error}</Notice>}
  <div className="stroke-scene-layout">
   <nav className="stroke-scene-list" aria-label={t("stroke.scopes")}>{scopes.map(item=>{
    const Icon=sceneIcon(item);
    return <div key={item.id} className="stroke-scene-row" data-selected={scope?.id===item.id||undefined}><Button variant="ghost" className="stroke-scene-button" aria-label={item.name} title={item.name} aria-pressed={scope?.id===item.id} disabled={disabled} onClick={()=>{onScopeChange(item.id);setError("");}}><Icon aria-hidden="true" className="size-4 shrink-0"/><span className="stroke-scene-label">{item.name}</span></Button><Switch aria-label={t("stroke.scopeEnabled",{name:item.name})} title={t(item.enabled?"stroke.sceneEnabled":"stroke.sceneDisabled")} checked={item.enabled} disabled={disabled} onCheckedChange={enabled=>changeScene(item.id,{enabled})}/></div>;
   })}</nav>
   {scope?<div className="stroke-scene-content settings-stack">
    <div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0 space-y-1"><h3 className="settings-group-title break-words">{scope.name}</h3><p className="settings-help">{t(scope.kind==="explorer-folders"?"stroke.explorerSceneHint":scope.kind==="global"?"stroke.globalSceneHint":"stroke.applicationSceneHint")}</p></div></div>
    <div className="flex flex-wrap items-center gap-2"><Button icon={Plus} disabled={disabled} onClick={openAssignment}>{t("stroke.assignGesture")}</Button><Button icon={Settings} variant="ghost" disabled={disabled} onClick={()=>setSettingsOpen(true)}>{t("stroke.sceneSettings")}</Button><span className="text-sm text-muted-foreground">{t("stroke.sceneActionCount",{count:entries.length})}</span></div>
    <p className="settings-help">{t("stroke.drawInstruction",{button:t("stroke."+preferences.triggerButton)})}</p>
    <div className="stroke-scene-actions" role="list" aria-label={t("stroke.sceneGestures")}>
     <div className="stroke-action-list-heading" aria-hidden="true"><span>{t("settingsDesign.gestureAction")}</span><span>{t("stroke.actionGesture")}</span></div>
     {entries.map(({pack,action,overriddenBy})=>{
      const gesture=preferences.gestures.find(item=>item.id===action.gesture);
      return <div role="listitem" key={pack.id+"/"+action.id}><div role="group" aria-label={t("stroke.sceneAction",{name:action.name})} className="stroke-action-row">
       <div className="stroke-action-preview">{gesture?<StrokeGestureThumbnail gesture={gesture} color={preferences.pen.color}/>:<Mouse aria-hidden="true" className="size-6 text-muted-foreground"/>}</div>
       <div className="stroke-action-copy"><span className="block break-words font-medium">{action.name}</span><span className="block break-words text-sm text-muted-foreground">{pack.name}</span>{overriddenBy&&<span className="block text-sm text-muted-foreground">{t("stroke.overriddenAction",{name:overriddenBy})}</span>}</div>
       <div className="stroke-action-gesture"><CaptureOptionSelect stacked label={t("stroke.actionGesture")} accessibleLabel={t("stroke.gestureFor",{name:action.name})} value={action.gesture} items={gestures} disabled={disabled} onChange={gesture=>updateAction(pack.id,action.id,{gesture})}/></div>
       <div className="stroke-action-controls"><Switch aria-label={t("stroke.enableAction",{name:action.name})} checked={action.enabled} disabled={disabled} onCheckedChange={enabled=>updateAction(pack.id,action.id,{enabled})}/><Button icon={CodeXml} variant="ghost" aria-label={t("stroke.editNamedAction",{name:action.name})} title={t("stroke.editNamedAction",{name:action.name})} disabled={disabled} onClick={()=>{setEditor({packageId:pack.id,actionId:action.id});setError("");}}/></div>
      </div></div>;
     })}
     {!entries.length&&<p className="settings-help py-4">{t("stroke.emptyScene")}</p>}
    </div>
    <p className="settings-help">{t("stroke.sceneEditHint")}</p>
   </div>:<p className="settings-help">{t("stroke.emptyScenes")}</p>}
  </div>
  <Dialog open={settingsOpen&&!!scope} onOpenChange={setSettingsOpen}><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{t("stroke.sceneSettings")}</DialogTitle><DialogDescription>{t("stroke.sceneSettingsHint")}</DialogDescription></DialogHeader>{scope&&<div className="settings-stack">
   <Field><FieldLabel htmlFor="stroke-scene-name">{t("stroke.scopeName")}</FieldLabel><Input id="stroke-scene-name" value={scope.name} disabled={disabled} onChange={event=>changeScope({name:event.target.value})}/></Field>
   <SettingsOptions label={t("stroke.scopeKind")} value={scope.kind} items={[{value:"explorer-folders",label:t("stroke.explorerFolders")},{value:"application",label:t("stroke.application")},{value:"global",label:t("stroke.global")}]} disabled={disabled} onChange={kind=>changeScope({kind:kind as StrokeScope["kind"]})}/>
   {scope.kind==="application"&&<><Field><FieldLabel htmlFor="stroke-scene-path">{t("stroke.scopePath")}</FieldLabel><Input {...processDrop} id="stroke-scene-path" value={scope.processPath??""} disabled={disabled} onChange={event=>changeScope({processPath:event.target.value})}/></Field><StrokePathPicker disabled={disabled} onPick={target=>changeScope({processPath:target.processPath})}/></>}
   <details className="space-y-3"><summary className="cursor-pointer text-sm font-medium">{t("stroke.advancedScope")}</summary>
    {scope.kind==="application"&&<><Field><FieldLabel htmlFor="stroke-scene-pattern">{t("stroke.scopePattern")}</FieldLabel><Textarea {...patternDrop} id="stroke-scene-pattern" value={scope.pattern??""} disabled={disabled} onChange={event=>changeScope({pattern:event.target.value})}/></Field><p className="settings-help">{t("stroke.pathRuleHint")}</p><StrokePathPicker disabled={disabled} onPick={target=>changeScope({processPath:"",pattern:[scope.pattern,escapedProcessPattern(target.processPath)].filter(Boolean).join("\n")})}/></>}
    <Field><FieldLabel htmlFor="stroke-scene-classes">{t("stroke.windowClasses")}</FieldLabel><Textarea id="stroke-scene-classes" value={scope.windowClasses?.join("\n")??""} disabled={disabled} onChange={event=>changeScope({windowClasses:event.target.value.split(/\r?\n/)})}/></Field>
    <fieldset className="space-y-2"><legend className="mb-2 text-sm font-medium">{t("stroke.scopePackages")}</legend><p className="settings-help">{t("stroke.packageOrder")}</p>{preferences.actionPackages.map(pack=><label key={pack.id} className="flex items-center gap-2 text-sm"><Checkbox checked={scope.packages.includes(pack.id)} disabled={disabled} onCheckedChange={checked=>changeScope({packages:checked?[...scope.packages,pack.id]:scope.packages.filter(id=>id!==pack.id)})}/><span className="min-w-0 break-words">{pack.name}</span></label>)}</fieldset>
   </details>
   {error&&<Notice>{error}</Notice>}
   <div className="flex flex-wrap justify-between gap-2"><Button icon={Trash2} variant="ghost" disabled={disabled} onClick={()=>{onChange({scopes:preferences.scopes.filter(item=>item.id!==scope.id)});setSettingsOpen(false);onScopeChange(scopes.find(item=>item.id!==scope.id)?.id??"");}}>{t("stroke.removeScope")}</Button><Button onClick={()=>setSettingsOpen(false)}>{t("stroke.doneEditing")}</Button></div>
  </div>}</DialogContent></Dialog>
  <Dialog open={!!editor&&!!editingAction} onOpenChange={open=>{if(!open)setEditor(undefined);}}><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl"><DialogHeader><DialogTitle>{t("stroke.editAction")}</DialogTitle><DialogDescription>{t("stroke.sceneEditHint")}</DialogDescription></DialogHeader>{editingAction&&editor&&<div className="settings-stack">
   <Field><FieldLabel htmlFor="stroke-scene-action-name">{t("stroke.actionName")}</FieldLabel><Input id="stroke-scene-action-name" value={editingAction.name} disabled={disabled} onChange={event=>updateAction(editor.packageId,editor.actionId,{name:event.target.value})}/></Field>
   <CaptureOptionSelect stacked label={t("stroke.actionGesture")} value={editingAction.gesture} items={gestures} disabled={disabled} onChange={gesture=>updateAction(editor.packageId,editor.actionId,{gesture})}/>
   {preferences.gestures.some(gesture=>gesture.id===editingAction.gesture)&&<Button variant="ghost" disabled={disabled} onClick={()=>{onManageGesture(editingAction.gesture);setEditor(undefined);}}>{t("stroke.manageTrail")}</Button>}
   <Field><FieldLabel htmlFor="stroke-scene-action-code">{t("stroke.actionCode")}</FieldLabel><Textarea id="stroke-scene-action-code" className="min-h-64 font-mono text-sm" value={editingAction.code} disabled={disabled} onChange={event=>updateAction(editor.packageId,editor.actionId,{code:event.target.value})}/></Field><p className="settings-help">{t("stroke.markHint")}</p><p className="settings-help">{t("stroke.scriptHint")}</p>
   {error&&<Notice>{error}</Notice>}<div className="flex flex-wrap justify-between gap-2"><Button icon={Trash2} variant="ghost" disabled={disabled} onClick={removeAction}>{t("stroke.removeSceneAction")}</Button><Button onClick={()=>setEditor(undefined)}>{t("stroke.doneEditing")}</Button></div>
  </div>}</DialogContent></Dialog>
  <Dialog open={!!assignment} onOpenChange={open=>{if(!open)setAssignment(undefined);}}><DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{t("stroke.assignGesture")}</DialogTitle><DialogDescription>{t("stroke.assignGestureHint")}</DialogDescription></DialogHeader>{assignment&&<div className="settings-stack">
   <CaptureOptionSelect stacked label={t("stroke.chooseAction")} value={assignment.source} items={[...sources,{value:"custom",label:t("stroke.customAction")}]} disabled={disabled} onChange={source=>{const action=sources.find(item=>item.value===source)?.action;setAssignment({source,name:action?.name??t("stroke.newAction"),gesture:action?.gesture??assignment.gesture,code:action?.code??""});}}/>
   <Field><FieldLabel htmlFor="stroke-assignment-name">{t("stroke.actionName")}</FieldLabel><Input id="stroke-assignment-name" value={assignment.name} disabled={disabled} onChange={event=>setAssignment({...assignment,name:event.target.value})}/></Field>
   <CaptureOptionSelect stacked label={t("stroke.actionGesture")} value={assignment.gesture} items={gestures} disabled={disabled} onChange={gesture=>setAssignment({...assignment,gesture})}/>
   {preferences.gestures.filter(gesture=>gesture.id===assignment.gesture).map(gesture=><div key={gesture.id} className="flex items-center gap-3"><StrokeGestureThumbnail gesture={gesture} color={preferences.pen.color}/><p className="settings-help">{t("stroke.assignmentPreview",{button:t("stroke."+preferences.triggerButton),gesture:gesture.name,action:assignment.name})}</p></div>)}
   <details open={assignment.source==="custom"}><summary className="cursor-pointer text-sm">{t("stroke.actionCode")}</summary><Textarea aria-label={t("stroke.actionCode")} className="mt-3 min-h-48 font-mono text-sm" value={assignment.code} disabled={disabled} onChange={event=>setAssignment({...assignment,code:event.target.value})}/><p className="settings-help mt-2">{t("stroke.scriptHint")}</p></details>
   {error&&<Notice>{error}</Notice>}<div className="flex flex-wrap justify-end gap-2"><Button variant="ghost" onClick={()=>setAssignment(undefined)}>{t("stroke.cancelAssignment")}</Button><Button disabled={disabled||!assignment.name.trim()} onClick={addAssignment}>{t("stroke.addToScene")}</Button></div>
  </div>}</DialogContent></Dialog>
 </div>;
}

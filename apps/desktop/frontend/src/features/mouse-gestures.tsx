import {usePathDrop} from "../ui/path-drop";
import {SettingsSaveBar} from "../components/settings-save-bar";
import {useEffect,useState,type ReactNode} from "react";
import {ArrowDown,ArrowUp,CodeXml,FolderPlus,MousePointer2,Paintbrush,Plus,RotateCcw,Save,ScanLine,Settings,ShieldOff,SlidersHorizontal,Trash2} from "lucide-react";
import {strokeCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Switch} from "../ui/switch";
import {Input} from "../components/ui/input";
import {Textarea} from "../components/ui/textarea";
import {Field,FieldLabel} from "../components/ui/field";
import {SettingsSection,SettingsOptions} from "../components/settings-layout";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {IconLabel} from "../ui/icon-label";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {StrokeGesturePreview,StrokeRecorder} from "./mouse-gestures-canvas";
import {escapedProcessPattern,StrokeContextMenu,StrokePathPicker} from "./mouse-gestures-controls";
import {StrokeScenes} from "./mouse-gestures-scenes";
import {orderedStrokeScopes} from "./mouse-gestures-scenes-model";
import {strokeId,type StrokeAction,type StrokeGesture,type StrokePackage,type StrokePreferences} from "./mouse-gestures-types";

// Keep trailing newlines while typing. Normalize only at the save boundary so
// Enter can create a new namespace, reference, exclusion or window-class row.
const lines=(value:string)=>value.split(/\r?\n/);
const committedLines=(value:string[])=>value.map(line=>line.trim()).filter(Boolean);
function TextField({id,label,children}:{id:string;label:string;children:ReactNode}){return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel>{children}</Field>;}
const fieldGrid="settings-field-grid";

// Stroke.Configure's action tree, gesture/pen/compiler/filter/thread editors
// live in the existing WinOne settings shell. Only this form owns the draft;
// the Host commits it and applies a complete worker snapshot transactionally.
export function MouseGesturesSettings({disabled=false,onActivity}:{disabled?:boolean;onActivity?:PageActivityChanged}){
 const {t}=useTranslation();const [draft,setDraft]=useState<StrokePreferences>(),[saved,setSaved]=useState<StrokePreferences>();
 const [busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [tab,setTab]=useState("scopes"),[selection,setSelection]=useState<{package:string;action?:string}>({package:""}),[gestureId,setGestureId]=useState(""),[recording,setRecording]=useState(false),[scopeId,setScopeId]=useState(""),[sceneInteraction,setSceneInteraction]=useState(false);
 const dirty=!!draft&&JSON.stringify(draft)!==JSON.stringify(saved),locked=busy||loading;
 const assembliesDrop=usePathDrop({kind:"file",multiple:true,disabled:locked,onError:setError,onPaths:paths=>change({assemblies:[...new Set([...(draft?.assemblies??[]),...paths])]})});
 const exclusionsDrop=usePathDrop({kind:"file",multiple:true,disabled:locked,onError:setError,onPaths:paths=>change({filters:[...new Set([...(draft?.filters??[]),...paths.map(escapedProcessPattern)])]})});
 usePageActivity(onActivity,dirty,busy||recording||sceneInteraction,"mouse-gestures",()=>{setDraft(saved);setError("");setNotice("");});
 useEffect(()=>{let live=true;void strokeCall<StrokePreferences>("settings.get").then(value=>{if(!live)return;setDraft(value);setSaved(value);setSelection({package:value.actionPackages[0]?.id??"",action:value.actionPackages[0]?.actions[0]?.id});setGestureId(value.gestures[0]?.id??"");setScopeId(orderedStrokeScopes(value.scopes)[0]?.id??"");},error=>{if(live)setError(String(error));}).finally(()=>{if(live)setLoading(false);});return()=>{live=false;};},[]);
 function change(patch:Partial<StrokePreferences>){setDraft(value=>value?{...value,...patch}:value);setNotice("");}
 function changePackage(id:string,patch:Partial<StrokePackage>){if(draft)change({actionPackages:draft.actionPackages.map(pack=>pack.id===id?{...pack,...patch}:pack)});}
 function changeAction(pack:StrokePackage,id:string,patch:Partial<StrokeAction>){changePackage(pack.id,{actions:pack.actions.map(action=>action.id===id?{...action,...patch}:action)});}
 function changeGesture(id:string,patch:Partial<StrokeGesture>){if(draft)change({gestures:draft.gestures.map(g=>g.id===id?{...g,...patch}:g)});}
 function addPackage(){if(!draft)return;const id=strokeId();change({actionPackages:[...draft.actionPackages,{id,name:t("stroke.newPackage"),actions:[]}]});setSelection({package:id});}
 function removePackage(id:string){if(!draft)return;const packages=draft.actionPackages.filter(pack=>pack.id!==id);change({actionPackages:packages,scopes:draft.scopes.map(scope=>({...scope,packages:scope.packages.filter(value=>value!==id)}))});setSelection({package:packages[0]?.id??""});}
 function addAction(pack:StrokePackage){const id=strokeId();changePackage(pack.id,{actions:[...pack.actions,{id,name:t("stroke.newAction"),gesture:draft?.gestures[0]?.id??"#1",code:"",enabled:true}]});setSelection({package:pack.id,action:id});}
 function removeAction(pack:StrokePackage,id:string){changePackage(pack.id,{actions:pack.actions.filter(action=>action.id!==id)});setSelection({package:pack.id});}
 function movePackage(id:string,offset:number){if(!draft)return;const items=[...draft.actionPackages],index=items.findIndex(p=>p.id===id);if(index+offset<0||index+offset>=items.length)return;[items[index],items[index+offset]]=[items[index+offset],items[index]];change({actionPackages:items});}
 function addGesture(){if(!draft)return;const id=strokeId();change({gestures:[...draft.gestures,{id,name:t("stroke.newGesture"),points:[],vectors:[]}]});setGestureId(id);}
 function removeGesture(id:string){if(!draft)return;const gestures=draft.gestures.filter(g=>g.id!==id);change({gestures,actionPackages:draft.actionPackages.map(pack=>({...pack,actions:pack.actions.filter(action=>action.gesture!==id)}))});setGestureId(gestures[0]?.id??"");}
 async function save(){if(!draft)return;setBusy(true);setError("");setNotice("");try{const value=await strokeCall<StrokePreferences>("settings.save",{...draft,namespaces:committedLines(draft.namespaces),assemblies:committedLines(draft.assemblies),filters:committedLines(draft.filters),scopes:draft.scopes.map(scope=>({...scope,windowClasses:scope.windowClasses?committedLines(scope.windowClasses):undefined}))});setDraft(value);setSaved(value);setNotice(t("stroke.saved"));}catch(error){setError(String(error));}finally{setBusy(false);}}
 const pack=draft?.actionPackages.find(p=>p.id===selection.package),action=pack?.actions.find(a=>a.id===selection.action),gesture=draft?.gestures.find(g=>g.id===gestureId);
 const menuItems=(pack:StrokePackage,action?:StrokeAction)=>[{label:t("stroke.addPackage"),run:addPackage,disabled:locked},{label:t("stroke.addAction"),run:()=>addAction(pack),disabled:locked},{label:t(action?"stroke.removeAction":"stroke.removePackage"),run:()=>action?removeAction(pack,action.id):removePackage(pack.id),disabled:locked}];
 return <section className="settings-stack"><p className="settings-help">{t("stroke.scopeHint")}</p>{loading&&<p role="status">{t("stroke.loading")}</p>}{error&&!draft&&<Notice>{error}</Notice>}
  {draft&&<><Tabs value={tab} onValueChange={value=>setTab(String(value))}><TabsList variant="line" className="h-auto max-w-full flex-wrap" aria-label={t("stroke.title")}>
   {([['scopes',ScanLine],['gestures',MousePointer2],['pen',Paintbrush],['filters',ShieldOff],['advancedSettings',Settings]] as const).map(([name,icon])=><TabsTrigger key={name} title={t("stroke."+name)} value={name} disabled={busy||recording||sceneInteraction}><IconLabel icon={icon}>{t("stroke."+name)}</IconLabel></TabsTrigger>)}
  </TabsList>
  {error&&<Notice>{error}</Notice>}{notice&&<p role="status" className="settings-help">{notice}</p>}
  <TabsContent value="scopes" className="settings-stack"><StrokeScenes preferences={draft} scopeId={scopeId} onScopeChange={setScopeId} disabled={locked} onChange={change} onInteractionChange={setSceneInteraction} onManageGesture={id=>{setGestureId(id);setTab("gestures");}}/></TabsContent>

  <TabsContent value="gestures" className="settings-stack"><div className="flex flex-wrap gap-2"><Button icon={Plus} disabled={locked} onClick={addGesture}>{t("stroke.addGesture")}</Button>{gesture&&<Button icon={Trash2} variant="ghost" disabled={locked} title={t("stroke.deleteReferences")} onClick={()=>removeGesture(gesture.id)}>{t("stroke.removeGesture")}</Button>}</div>
   <div className="grid gap-5 sm:grid-cols-[12rem_minmax(0,1fr)]"><nav className="max-h-96 space-y-1 overflow-auto" aria-label={t("stroke.gestures")}>{draft.gestures.map(gesture=><StrokeContextMenu key={gesture.id} items={[{label:t("stroke.addGesture"),run:addGesture,disabled:locked},{label:t("stroke.removeGesture"),run:()=>removeGesture(gesture.id),disabled:locked}]}><button type="button" className={`block w-full rounded px-3 py-2 text-left hover:bg-muted ${gesture.id===gestureId?"bg-muted":""}`} onClick={()=>setGestureId(gesture.id)}>{gesture.name}</button></StrokeContextMenu>)}</nav>{gesture&&<div className="min-w-0 space-y-4"><TextField id="stroke-gesture-name" label={t("stroke.gestureName")}><Input id="stroke-gesture-name" value={gesture.name} disabled={locked} onChange={event=>changeGesture(gesture.id,{name:event.target.value.replace(/^#+/,"")})}/></TextField><StrokeGesturePreview gesture={gesture} color={draft.pen.color} disabled={locked||disabled} onRecord={()=>setRecording(true)} onReset={()=>changeGesture(gesture.id,{points:[],vectors:[]})}/><div className="flex flex-wrap gap-2"><Button icon={MousePointer2} disabled={locked||disabled} onClick={()=>setRecording(true)}>{t("stroke.record")}</Button><Button icon={RotateCcw} variant="ghost" disabled={locked} onClick={()=>changeGesture(gesture.id,{points:[],vectors:[]})}>{t("stroke.resetGesture")}</Button></div><p className="settings-help">{t(disabled?"stroke.recordDisabled":"stroke.gestureHint")}</p><p className="settings-help">{t("stroke.blankAllowed")}</p></div>}</div>
  </TabsContent>
  <TabsContent value="pen" className="settings-section-stack"><SettingsSection icon={MousePointer2} title={t("stroke.trigger")} headingId="stroke-trigger-heading"><SettingsOptions label={t("stroke.trigger")} labelledBy="stroke-trigger-heading" value={draft.triggerButton} disabled={locked} items={["middle","left","right","x1","x2"].map(value=>({value,label:t("stroke."+value)}))} onChange={triggerButton=>change({triggerButton})}/></SettingsSection><SettingsSection icon={Paintbrush} title={t("stroke.pen")}><div className="settings-switch-row"><label htmlFor="stroke-pen-enabled">{t("stroke.penEnabled")}</label><Switch id="stroke-pen-enabled" checked={draft.pen.enabled!==false} disabled={locked} onCheckedChange={enabled=>change({pen:{...draft.pen,enabled}})}/></div><div className={fieldGrid}>
   <TextField id="stroke-color" label={t("stroke.penColor")}><Input id="stroke-color" type="color" className="max-w-28" value={draft.pen.color} disabled={locked} onChange={event=>change({pen:{...draft.pen,color:event.target.value}})}/></TextField>
   <TextField id="stroke-opacity" label={t("stroke.penOpacity")}><Input id="stroke-opacity" type="number" min={0} max={100} value={Math.round(draft.pen.opacity*100)} disabled={locked} onChange={event=>change({pen:{...draft.pen,opacity:Number(event.target.value)/100}})}/></TextField>
   <TextField id="stroke-width" label={t("stroke.penWidth")}><Input id="stroke-width" type="number" min={0} max={10} value={draft.pen.thickness} disabled={locked} onChange={event=>change({pen:{...draft.pen,thickness:Number(event.target.value)}})}/></TextField>
  </div><p className="settings-help">{t("stroke.penHint")}</p></SettingsSection></TabsContent>

  <TabsContent value="filters" className="settings-stack"><p className="settings-help">{t("stroke.excludeHint")}</p><TextField id="stroke-exclusions" label={t("stroke.excludePatterns")}><Textarea {...exclusionsDrop} id="stroke-exclusions" className="min-h-48 font-mono text-sm" value={draft.filters.join("\n")} disabled={locked} onChange={event=>change({filters:lines(event.target.value)})}/></TextField><StrokePathPicker disabled={locked} onPick={target=>change({filters:[...draft.filters,escapedProcessPattern(target.processPath)]})}/></TabsContent>

  <TabsContent value="advancedSettings"><Tabs defaultValue="actions"><TabsList variant="line" aria-label={t("stroke.advancedSettings")}>{([["actions",CodeXml],["compile",Settings],["threads",SlidersHorizontal]] as const).map(([name,icon])=><TabsTrigger key={name} value={name}><IconLabel icon={icon}>{t("stroke."+name)}</IconLabel></TabsTrigger>)}</TabsList><TabsContent value="actions" className="settings-stack stroke-packages"><div className="flex flex-wrap items-center gap-2"><Button icon={FolderPlus} disabled={locked} onClick={addPackage}>{t("stroke.addPackage")}</Button>{pack&&<Button icon={Plus} disabled={locked} onClick={()=>addAction(pack)}>{t("stroke.addAction")}</Button>}<p className="settings-help">{t("stroke.packageOrder")}</p></div>
   <div className="stroke-package-layout"><nav className="stroke-package-tree" aria-label={t("stroke.actions")}>
    {draft.actionPackages.map(pack=><details key={pack.id} open className="min-w-0"><summary className="stroke-package-summary"><StrokeContextMenu inline items={menuItems(pack)}><button type="button" className="stroke-package-name" title={pack.name} aria-pressed={selection.package===pack.id&&!selection.action} onClick={event=>{event.preventDefault();setSelection({package:pack.id});}}>{pack.name}</button></StrokeContextMenu></summary><div className="space-y-1 pl-4">{pack.actions.map(action=><StrokeContextMenu key={action.id} items={menuItems(pack,action)}><button type="button" title={action.name} className={`stroke-package-action block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted ${selection.action===action.id&&selection.package===pack.id?"bg-muted":""}`} aria-pressed={selection.action===action.id&&selection.package===pack.id} onClick={()=>setSelection({package:pack.id,action:action.id})}>{action.name}</button></StrokeContextMenu>)}</div></details>)}
   </nav><div className="min-w-0 space-y-4">{pack?(action?<>
    <TextField id="stroke-action-name" label={t("stroke.actionName")}><Input id="stroke-action-name" value={action.name} disabled={locked} onChange={event=>changeAction(pack,action.id,{name:event.target.value})}/></TextField>
    <label className="flex items-center justify-between gap-3 text-sm"><span>{t("stroke.actionEnabled")}</span><Switch checked={action.enabled} disabled={locked} onCheckedChange={enabled=>changeAction(pack,action.id,{enabled})}/></label>
    <CaptureOptionSelect stacked label={t("stroke.actionGesture")} value={action.gesture} items={[...draft.gestures.map(gesture=>({value:gesture.id,label:gesture.name})),...Array.from({length:7},(_,index)=>({value:"#"+index,label:t("stroke.special"+index)}))]} disabled={locked} onChange={gesture=>changeAction(pack,action.id,{gesture})}/>
    <TextField id="stroke-code" label={t("stroke.actionCode")}><Textarea id="stroke-code" className="min-h-64 font-mono text-sm" value={action.code} disabled={locked} onChange={event=>changeAction(pack,action.id,{code:event.target.value})}/></TextField>
    <p className="settings-help">{t("stroke.markHint")}</p><p className="settings-help">{t("stroke.scriptHint")}</p><details><summary className="cursor-pointer text-sm">{t("stroke.advanced")}</summary><p className="settings-help py-2">{t("stroke.apiHint")}</p></details>
    <Button icon={Trash2} variant="ghost" disabled={locked} onClick={()=>removeAction(pack,action.id)}>{t("stroke.removeAction")}</Button>
   </>:<><TextField id="stroke-package-name" label={t("stroke.packageName")}><Input id="stroke-package-name" value={pack.name} disabled={locked} onChange={event=>changePackage(pack.id,{name:event.target.value})}/></TextField><p className="settings-help">{t("stroke.packageOrder")}</p><div className="flex flex-wrap gap-2"><Button icon={ArrowUp} disabled={locked||draft.actionPackages[0].id===pack.id} onClick={()=>movePackage(pack.id,-1)}>{t("stroke.moveUp")}</Button><Button icon={ArrowDown} disabled={locked||draft.actionPackages.at(-1)?.id===pack.id} onClick={()=>movePackage(pack.id,1)}>{t("stroke.moveDown")}</Button><Button icon={Trash2} variant="ghost" disabled={locked} onClick={()=>removePackage(pack.id)}>{t("stroke.removePackage")}</Button></div></>):<p className="settings-help">{t("stroke.emptyActions")}</p>}</div></div>
  </TabsContent>
<TabsContent value="compile" className="settings-stack"><p className="settings-help">{t("stroke.assemblyHint")}</p><TextField id="stroke-assemblies" label={t("stroke.assemblies")}><Textarea {...assembliesDrop} id="stroke-assemblies" className="min-h-40 font-mono text-sm" value={draft.assemblies.join("\n")} disabled={locked} onChange={event=>change({assemblies:lines(event.target.value)})}/></TextField><TextField id="stroke-namespaces" label={t("stroke.namespaces")}><Textarea id="stroke-namespaces" className="min-h-64 font-mono text-sm" value={draft.namespaces.join("\n")} disabled={locked} onChange={event=>change({namespaces:lines(event.target.value)})}/></TextField></TabsContent>
<TabsContent value="threads" className="settings-stack"><div className={fieldGrid}><TextField id="stroke-threads" label={t("stroke.threadCount")}><Input id="stroke-threads" type="number" min={1} max={64} value={draft.scriptThreadCount} disabled={locked} onChange={event=>change({scriptThreadCount:Number(event.target.value)})}/></TextField><TextField id="stroke-timeout" label={t("stroke.actionTimeout")}><Input id="stroke-timeout" type="number" min={0} max={3600} value={draft.scriptTimeoutSeconds} disabled={locked} onChange={event=>change({scriptTimeoutSeconds:Number(event.target.value)})}/></TextField></div><p className="settings-help">{t("stroke.threadHint")}</p></TabsContent></Tabs></TabsContent>
  </Tabs><SettingsSaveBar undo={{disabled:locked||!dirty,onClick:()=>{setDraft(saved);setError("");setNotice("");}}}><Button icon={Save} variant="primary" busy={busy} disabled={locked||!dirty} onClick={()=>void save()}>{t("stroke.save")}</Button></SettingsSaveBar>
  {recording&&gesture&&<StrokeRecorder gesture={gesture} preferences={draft} onUse={value=>changeGesture(gesture.id,value)} onClose={()=>setRecording(false)}/>}
  </>}
 </section>;
}

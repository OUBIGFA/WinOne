import {usePathDrop} from "../ui/path-drop";
import {Notice,InlineError} from "../ui/notice";
import {Checkbox} from "../components/ui/checkbox";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";
import {Upload} from "lucide-react";
import {SettingsSection} from "../components/settings-layout";
import { useState } from "react";
import { pickProfile, pickProfileLocation, profileCall } from "../bridge";
import { Button } from "../ui/button";
import { ProfileSettingsReview } from "./profile-settings-review";
import "./profile-import.css";
import { useTranslation } from "../i18n";
import { usePageActivity, type PageActivityChanged } from "../ui/page-navigation";

export interface ProfilePath { id:string; label:string; kind:string; value:string; state:string; error?:string }
interface ImportModule { id:string; state:string; requiredPaths:string[]; error?:string; settings?:Record<string,unknown> }
interface Preview {
  name:string; sourceHash:string; configurationRevision:number; moduleRevision:number;
  preferences:{theme:string;language:string;runAtLogon:boolean;startMinimized:boolean;defaultPage?:string;keepUIRunning?:boolean;openShortcut?:string;enabled:Record<string,boolean>;shortcuts:Record<string,string>};
  currentShortcuts:Record<string,string>;
  plan:{modules:ImportModule[];shortcutIssues:{action:string;kind:string;error:string}[]};
  paths:ProfilePath[];
}
const knownModules=new Set(["capture","always-on-top","wintray","selection","quick-assistant","stroke"]);
const knownActions=new Set(["capture.region","capture.ocr","capture.translate","always-on-top.toggle","selection.show","quick-assistant.show"]);
const languages:Record<string,string>={system:"shell.system","zh-CN":"profiles.chinese","en-US":"profiles.english"};
const status:Record<string,string>={ready:"profiles.ready",needs_mapping:"profiles.mapping",missing_module:"profiles.missing",invalid:"profiles.invalid"};

export function ProfileImport({onApplied,onActivity}:{onApplied:()=>void;onActivity?:PageActivityChanged}) {
  const {t}=useTranslation();
  const moduleName=(id:string)=>id==="stroke"?t("stroke.title"):knownModules.has(id)?t(`nav.${id}`):id;
  const pageName=(id:string)=>t(id==="chat"?"chat.title":id==="ai-services"?"ai.serviceTitle":`nav.${id||"overview"}`);
  const pathName=(path:ProfilePath)=>path.id==="capture.output"?t("profiles.captureFolder"):(path.label||t("profiles.programLocation"));
  const [source,setSource]=useState("");
  const [view,setView]=useState("overview"),[detailModule,setDetailModule]=useState("");
  const [preview,setPreview]=useState<Preview>();
  const [bindings,setBindings]=useState<Record<string,string>>({});
  const [selected,setSelected]=useState<string[]>([]);
  const [global,setGlobal]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [notice,setNotice]=useState("");
  const [needsReview,setNeedsReview]=useState(false);
  // A reviewed import contains choices and local mappings that must survive
  // cancelled navigation. Applying clears them only after the host confirms.
  usePageActivity(onActivity,!!preview,busy,"profile-import",()=>{setPreview(undefined);setNeedsReview(false);setError("");setNotice("");});
  async function review(path:string,mappings:Record<string,string>,reset=false) {
    const next=await profileCall<Preview>("import.preview",{path,bindings:mappings});
    setPreview(next);setSource(path);setBindings(mappings);setNeedsReview(false);
    setSelected(current=>reset?next.plan.modules.filter(m=>m.state!=="invalid").map(m=>m.id):current.filter(id=>next.plan.modules.some(m=>m.id===id)));
    if(reset){setGlobal(true);setView("overview");setDetailModule("");}
  }
  const archiveDrop=usePathDrop({kind:"file",disabled:busy,onPaths:paths=>choose(paths[0]),onError:setError});
  async function choose(chosen?:string) {
    if(busy)return;
    setBusy(true);setError("");setNotice("");
    // Replace the current choices only after a new archive can be reviewed.
    // A failed replacement retains its mappings and can revalidate the old file.
    try { const path=chosen??await pickProfile();if(path){setNeedsReview(true);await review(path,{},true);} }
    catch(error){setError(String(error));}
    finally{setBusy(false);}
  }
  async function mapPath(path:ProfilePath,chosen?:string) {
    setBusy(true);setError("");
    try {const value=chosen??await pickProfileLocation(path.kind);if(value){const next={...bindings,[path.id]:value};setBindings(next);setNeedsReview(true);await review(source,next);}}
    catch(error){setError(String(error));setNeedsReview(true);}
    finally{setBusy(false);}
  }
  async function refresh() {
    setBusy(true);setError("");
    try{await review(source,bindings);}catch(error){setError(String(error));setNeedsReview(true);}finally{setBusy(false);}
  }
  async function apply() {
    if(!preview)return;
    setBusy(true);setError("");setNotice("");
    try {
      await profileCall("import.apply",{path:source,bindings,sourceHash:preview.sourceHash,configurationRevision:preview.configurationRevision,moduleRevision:preview.moduleRevision,modules:selected,global});
      setPreview(undefined);setNotice("profiles.imported");onApplied();
    } catch(error){setError(String(error));setNeedsReview(true);}
    finally{setBusy(false);}
  }
  const chosen=preview?.plan.modules.filter(module=>selected.includes(module.id))??[];
  const required=new Set(chosen.flatMap(module=>module.requiredPaths??[]));
  const paths=preview?.paths.filter(path=>required.has(path.id))??[];
  // The archive-wide conflict list includes unselected values. Recompute
  // conflicts below against exactly what this import keeps and replaces.
  const issues=preview?.plan.shortcutIssues.filter(issue=>!["missing_module","conflict"].includes(issue.kind)&&(selected.some(id=>issue.action.startsWith(`${id}.`))||(global&&issue.action==="application.open"))).map(issue=>issue.error)??[];
  const keys={...preview?.currentShortcuts};
  if(global&&preview?.preferences.openShortcut!=null)keys["application.open"]=preview.preferences.openShortcut;
  for(const id of selected) {
    for(const [action,value] of Object.entries(preview?.preferences.shortcuts??{})) {
      if(!action.startsWith(`${id}.`)||!knownModules.has(id))continue;
      if(!knownActions.has(action))issues.push(t("profiles.unsupported",{name:moduleName(id)}));
      else keys[action]=value;
    }
  }
  const canonical=(value:string|undefined)=>(value??"").trim().toUpperCase().split("+").map(part=>part.trim()).sort().join("+");
  const usedKeys=new Set<string>();
  for(const value of Object.values(keys)){const key=canonical(value);if(!key)continue;if(usedKeys.has(key))issues.push(t("profiles.conflict"));usedKeys.add(key);}
  const details=chosen.filter(module=>module.settings);
  const detail=details.find(module=>module.id===detailModule)??details[0];
  const blocked=needsReview||(!global&&!selected.length)||chosen.some(module=>!["ready","missing_module"].includes(module.state))||paths.some(path=>path.state!=="ready")||issues.length>0;
  return <SettingsSection {...archiveDrop} className="profile-import" icon={Upload} title={t("profiles.import")} description={t("profiles.importHint")}>
    <div className="settings-action-row"><Button icon={Upload} busy={busy&&!preview} disabled={busy} onClick={()=>void choose()}>{t("profiles.chooseFile")}</Button></div>
    {preview&&<div className="profile-import__review">
      <div className="settings-stack-compact"><h3>{preview.name||t("profiles.import")}</h3>
      <p className="break-all text-sm text-muted-foreground">{source}</p></div>
      <fieldset className="plain-fieldset" disabled={busy}>
        <Tabs navigation={false} value={view} onValueChange={value=>setView(String(value))}><TabsList variant="line" aria-label={t("profiles.import")}><TabsTrigger value="overview">{t("profiles.contents")}</TabsTrigger><TabsTrigger value="details">{t("profiles.details")}</TabsTrigger><TabsTrigger value="locations">{t("profiles.localPaths")}</TabsTrigger></TabsList><TabsContent value="overview">
        <label className="check-row"><Checkbox checked={global} onCheckedChange={setGlobal}/>{t("profiles.global")}</label>
        {global&&<div className="muted">
          <p>{t(`shell.${preview.preferences.theme}`)} · {t(languages[preview.preferences.language])} · {t("profiles.signIn",{value:t(preview.preferences.runAtLogon?"profiles.on":"profiles.off")})} · {t("profiles.hidden",{value:t(preview.preferences.startMinimized?"profiles.on":"profiles.off")})}</p>
          <p>{t("profiles.defaultPage",{value:preview.preferences.defaultPage==null?t("profiles.keepCurrent"):pageName(preview.preferences.defaultPage)})}</p>
          <p>{t("profiles.keepUI",{value:preview.preferences.keepUIRunning==null?t("profiles.keepCurrent"):t(preview.preferences.keepUIRunning?"profiles.on":"profiles.off")})}</p>
          <p>{t("profiles.openShortcut",{value:preview.preferences.openShortcut==null?t("profiles.keepCurrent"):preview.preferences.openShortcut||t("profiles.unset")})}</p>
        </div>}
        {preview.plan.modules.map(module=><div className="profile-import__module" key={module.id}>
          <label className="check-row"><Checkbox checked={selected.includes(module.id)} onCheckedChange={checked=>setSelected(current=>checked?[...new Set([...current,module.id])]:current.filter(id=>id!==module.id))}/>{moduleName(module.id)}</label>
          <p className="muted">{module.error||t(status[module.state])}{selected.includes(module.id)&&typeof preview.preferences.enabled[module.id]==="boolean"&&module.state!=="missing_module"?` · ${t(preview.preferences.enabled[module.id]?"profiles.enabledAfter":"profiles.disabledAfter")}`:""}</p>
          {selected.includes(module.id)&&module.id==="wintray"&&Array.isArray(module.settings?.managedApps)&&<p>{t("profiles.replace",{count:module.settings.managedApps.length})}</p>}

        </div>)}
        </TabsContent><TabsContent value="details" className="space-y-4 py-4">
          {detail?<>
            <Label htmlFor="profile-detail-module">{t("profiles.module")}</Label>
            <Select value={detail.id} items={details.map(module=>({value:module.id,label:moduleName(module.id)}))} onValueChange={value=>{if(value)setDetailModule(value);}}>
              <SelectTrigger id="profile-detail-module"><SelectValue/></SelectTrigger><SelectContent>{details.map(module=><SelectItem key={module.id} value={module.id}>{moduleName(module.id)}</SelectItem>)}</SelectContent>
            </Select>
            <ProfileSettingsReview values={detail.settings!} paths={preview.paths}/>
            {Object.entries(preview.preferences.shortcuts).filter(([action])=>action.startsWith(`${detail.id}.`)).map(([action,key])=><p key={action}>{t("profiles.shortcut",{key:key||t("profiles.unset")})}</p>)}
          </>:<p className="text-muted-foreground">{t("profiles.noDetails")}</p>}
        </TabsContent><TabsContent value="locations">
        {!paths.length&&<p className="py-4 text-muted-foreground">{t("profiles.noPaths")}</p>}
        {paths.length>0&&<h3>{t("profiles.locations")}</h3>}
        {paths.map(path=><ProfilePathRow key={path.id} path={path} name={pathName(path)} disabled={busy} onChoose={value=>mapPath(path,value)} onError={setError}/>)}
        </TabsContent></Tabs>
      </fieldset>
      {view!=="locations"&&paths.some(path=>path.state!=="ready")&&<Button disabled={busy} onClick={()=>setView("locations")}>{t("profiles.fixPaths")}</Button>}
      {issues.length>0&&<Notice><ul className="list-disc space-y-1 pl-4">{[...new Set(issues)].map(issue=><li key={issue}>{issue}</li>)}</ul></Notice>}
      <div className="detail-actions"><Button variant="primary" busy={busy} disabled={busy||blocked} onClick={()=>void apply()}>{t("profiles.confirm")}</Button><Button disabled={busy} onClick={()=>void refresh()}>{t("profiles.review")}</Button><Button variant="ghost" disabled={busy} onClick={()=>{setPreview(undefined);setError("");}}>{t("profiles.cancel")}</Button></div>
    </div>}
    {error&&<Notice>{error}</Notice>}
    <p role="status">{notice?t(notice):""}</p>
  </SettingsSection>;
}

function ProfilePathRow({path,name,disabled,onChoose,onError}:{path:ProfilePath;name:string;disabled:boolean;onChoose:(value?:string)=>Promise<void>;onError:(error:string)=>void}){
 const {t}=useTranslation();
 const drop=usePathDrop({kind:path.kind==="directory"?"directory":"file",disabled,onPaths:paths=>onChoose(paths[0]),onError});
 return <div {...drop} className="setting-row"><div><span>{name}</span><p className="program-path">{path.value||t("profiles.noLocation")}</p>{path.error&&<InlineError>{path.error}</InlineError>}</div><Button disabled={disabled} aria-label={t("profiles.chooseFor",{name})} onClick={()=>void onChoose()}>{t("profiles.choose")}</Button></div>;
}

import {Notice,InlineError} from "../ui/notice";
import {Checkbox} from "../components/ui/checkbox";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";
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
  preferences:{theme:string;language:string;runAtLogon:boolean;startMinimized:boolean;enabled:Record<string,boolean>;shortcuts:Record<string,string>};
  currentShortcuts:Record<string,string>;
  plan:{modules:ImportModule[];shortcutIssues:{action:string;kind:string;error:string}[]};
  paths:ProfilePath[];
}
const knownModules=new Set(["capture","always-on-top","wintray"]);
const languages:Record<string,string>={system:"shell.system","zh-CN":"profiles.chinese","en-US":"profiles.english"};
const status:Record<string,string>={ready:"profiles.ready",needs_mapping:"profiles.mapping",missing_module:"profiles.missing",invalid:"profiles.invalid"};

export function ProfileImport({onApplied,onActivity}:{onApplied:()=>void;onActivity?:PageActivityChanged}) {
  const {t}=useTranslation();
  const moduleName=(id:string)=>knownModules.has(id)?t(`nav.${id}`):id;
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
  usePageActivity(onActivity,!!preview,busy,"profile-import");
  async function review(path:string,mappings:Record<string,string>,reset=false) {
    const next=await profileCall<Preview>("import.preview",{path,bindings:mappings});
    setPreview(next);setSource(path);setBindings(mappings);setNeedsReview(false);
    setSelected(current=>reset?next.plan.modules.filter(m=>m.state!=="invalid").map(m=>m.id):current.filter(id=>next.plan.modules.some(m=>m.id===id)));
    if(reset){setGlobal(true);setView("overview");setDetailModule("");}
  }
  async function choose() {
    setBusy(true);setError("");setNotice("");
    // Replace the current choices only after a new archive can be reviewed.
    // A failed replacement retains its mappings and can revalidate the old file.
    try { const path=await pickProfile();if(path){setNeedsReview(true);await review(path,{},true);} }
    catch(error){setError(String(error));}
    finally{setBusy(false);}
  }
  async function mapPath(path:ProfilePath) {
    setBusy(true);setError("");
    try {const value=await pickProfileLocation(path.kind);if(value){const next={...bindings,[path.id]:value};setBindings(next);setNeedsReview(true);await review(source,next);}}
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
  const issues=preview?.plan.shortcutIssues.filter(issue=>issue.kind!=="missing_module"&&selected.some(id=>issue.action.startsWith(`${id}.`))).map(issue=>issue.error)??[];
  const keys={...preview?.currentShortcuts};
  for(const id of selected) {
    for(const [action,value] of Object.entries(preview?.preferences.shortcuts??{})) {
      if(!action.startsWith(`${id}.`)||!knownModules.has(id))continue;
      if(!["capture.region","always-on-top.toggle"].includes(action))issues.push(t("profiles.unsupported",{name:moduleName(id)}));
      else keys[action]=value;
    }
  }
  const canonical=(value:string|undefined)=>(value??"").trim().toUpperCase().split("+").map(part=>part.trim()).sort().join("+");
  if(canonical(keys["capture.region"])&&canonical(keys["capture.region"])===canonical(keys["always-on-top.toggle"]))issues.push(t("profiles.conflict"));
  const details=chosen.filter(module=>module.settings);
  const detail=details.find(module=>module.id===detailModule)??details[0];
  const blocked=needsReview||(!global&&!selected.length)||chosen.some(module=>!["ready","missing_module"].includes(module.state))||paths.some(path=>path.state!=="ready")||issues.length>0;
  return <div className="profile-import">
    <div className="setting-row"><div><span>{t("profiles.import")}</span><p>{t("profiles.importHint")}</p></div><Button busy={busy&&!preview} disabled={busy} onClick={()=>void choose()}>{t("profiles.import")}</Button></div>
    {preview&&<div className="profile-import__review">
      <h3>{preview.name||t("profiles.import")}</h3>
      <p className="break-all text-sm text-muted-foreground">{source}</p>
      <fieldset className="plain-fieldset" disabled={busy}>
        <Tabs value={view} onValueChange={value=>setView(String(value))}><TabsList aria-label={t("profiles.import")}><TabsTrigger value="overview">{t("profiles.contents")}</TabsTrigger><TabsTrigger value="details">{t("profiles.details")}</TabsTrigger><TabsTrigger value="locations">{t("profiles.localPaths")}</TabsTrigger></TabsList><TabsContent value="overview">
        <label className="check-row"><Checkbox checked={global} onCheckedChange={setGlobal}/>{t("profiles.global")}</label>
        {global&&<p className="muted">{t(`shell.${preview.preferences.theme}`)} · {t(languages[preview.preferences.language])} · {t("profiles.signIn",{value:t(preview.preferences.runAtLogon?"profiles.on":"profiles.off")})} · {t("profiles.hidden",{value:t(preview.preferences.startMinimized?"profiles.on":"profiles.off")})}</p>}
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
        {paths.map(path=><div className="setting-row" key={path.id}><div><span>{pathName(path)}</span><p className="program-path">{path.value||t("profiles.noLocation")}</p>{path.error&&<InlineError>{path.error}</InlineError>}</div><Button aria-label={t("profiles.chooseFor",{name:pathName(path)})} onClick={()=>void mapPath(path)}>{t("profiles.choose")}</Button></div>)}
        </TabsContent></Tabs>
      </fieldset>
      {view!=="locations"&&paths.some(path=>path.state!=="ready")&&<Button disabled={busy} onClick={()=>setView("locations")}>{t("profiles.fixPaths")}</Button>}
      {issues.length>0&&<Notice><ul className="list-disc space-y-1 pl-4">{[...new Set(issues)].map(issue=><li key={issue}>{issue}</li>)}</ul></Notice>}
      <div className="detail-actions"><Button variant="primary" busy={busy} disabled={busy||blocked} onClick={()=>void apply()}>{t("profiles.confirm")}</Button><Button disabled={busy} onClick={()=>void refresh()}>{t("profiles.review")}</Button><Button variant="ghost" disabled={busy} onClick={()=>{setPreview(undefined);setError("");}}>{t("profiles.cancel")}</Button></div>
    </div>}
    {error&&<Notice>{error}</Notice>}
    <p role="status">{notice?t(notice):""}</p>
  </div>;
}

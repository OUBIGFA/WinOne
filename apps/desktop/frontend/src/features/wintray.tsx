import {usePathDrop} from "../ui/path-drop";
import {SettingsSaveBar} from "../components/settings-save-bar";
import {List,Power,Rocket,Save,Timer} from "lucide-react";
import {IconLabel} from "../ui/icon-label";
import {Notice,InlineError} from "../ui/notice";
import {readProgramIconCache,saveProgramIconCache} from "./program-icon-cache";
import { useEffect, useState } from "react";
import { pickProgram, pickPrograms, programInfo, wintrayCall } from "../bridge";

import { Button } from "../ui/button";
import { useTranslation } from "../i18n";
import {FolderOpen,AppWindow} from "lucide-react";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";
import "./wintray.css";
import {Switch} from "../components/ui/switch";
import {Label} from "../components/ui/label";
import {Item,ItemGroup} from "../components/ui/item";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import {WinTrayProgramActions} from './wintray-program-actions';
import {WinTrayProgramSettings,validProgramTiming} from "./wintray-program-settings";
import {NumberField,validNumber} from "../ui/number-field";
import {WinTrayMaintenance} from "./wintray-maintenance";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {SettingsSection} from "../components/settings-layout";

export interface Program {
  id: string;
  name: string;
  exePath: string;
  args: string;
  runOnStartup: boolean;
  collectTrayIcon: boolean;
  launchViaLogonTask: boolean;
  launchHiddenInBackground: boolean;
  trayBehavior: {
    autoMinimizeAndHideOnLaunch: boolean;
    closeDelaySeconds: number;
  };
  schedule: {
    enabled: boolean;
    startDelayMinutes: number;
    autoExitMinutes: number;
    frequencyEnabled: boolean;
    frequencyDays: number;
    frequencyRuns: number;
  };
}
interface TraySettings {
  schemaVersion: number;
  language: string;
  runAtLogon: boolean;
  startMinimizedToTray: boolean;
  exitAfterManagedAppsCompleted: boolean;
  closeWindowRetrySeconds: number;
  startupIntervalSeconds: number;
  managedApps: Program[];
}
interface TraySnapshot {
  revision: string;
  settings: TraySettings;
  recoveryError?: string;
  backgroundError?: string;
  startupError?: string;
}
interface Job {
  id: string;
  state: string;
  error?: string;
  result?: unknown;
}

export function WinTrayPage({ disabled = false,onChanged=()=>{},onActivity }: { disabled?: boolean;onChanged?:()=>void;onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [view,setView]=useState("programs");
  const [saved, setSaved] = useState<TraySnapshot>();
  const [draft, setDraft] = useState<TraySettings>();
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [dragged, setDragged] = useState("");
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next = await wintrayCall<TraySnapshot>(disabled ? "settings.get" : "snapshot");
      setSaved((current) =>
        preserveDraft && current
          ? {
              ...current,
              recoveryError: next.recoveryError,
              backgroundError: next.backgroundError,
              startupError: next.startupError,
            }
          : next,
      );
      if (!preserveDraft) { setSelected(current=>next.settings.managedApps.some(p=>p.id===current)?current:next.settings.managedApps[0]?.id??""); setDraft(next.settings); }
      if (next.recoveryError || next.backgroundError)
        setError(next.recoveryError || next.backgroundError || "");
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void reload();
  }, [disabled]);
  const apps = draft?.managedApps ?? [];
  const [metadata,setMetadata]=useState<Record<string,{name:string;icon?:string}>>(readProgramIconCache);
  useEffect(()=>saveProgramIconCache(metadata),[metadata]);
  const programPaths=JSON.stringify([...new Set(apps.map(program=>program.exePath))]);
  useEffect(()=>{
    let alive=true;
    const pending=JSON.parse(programPaths) as string[];let cursor=0;
    // Bound Shell reads so a long program list does not create a thread per row.
    async function load(){while(alive&&cursor<pending.length){const path=pending[cursor++];
      try{const info=await programInfo(path);if(alive)setMetadata(current=>({...current,[path]:info.icon?info:current[path]??info}));}
      catch{/* A temporarily unavailable file must not erase its cached icon. */}
    }}
    for(let worker=0;worker<Math.min(4,pending.length);worker++)void load();
    return()=>{alive=false;};
  },[programPaths]);
  // Saved/edited names are authoritative. Metadata seeds Add/Replace only;
  // a fallback keeps a temporarily empty draft identifiable in the list.
  const displayName=(program:Program)=>program.name.trim()?program.name:(metadata[program.exePath]?.name||program.exePath.split(/[\\/]/).pop()||t("wintray.name"));
  const active = apps.find((a) => a.id === selected);
  const invalidTimingSections=apps.filter(program=>!validProgramTiming(program)).map(displayName);
  if(draft&&(!validNumber(draft.startupIntervalSeconds,0,120)||!validNumber(draft.closeWindowRetrySeconds,0,120)))invalidTimingSections.push(t("wintray.advancedStartup"));
  const validNumbers=invalidTimingSections.length===0;
  const invalidNames=apps.filter(program=>!program.name.trim()).map(displayName);
  const validNames=invalidNames.length===0;
  const dirty =
    !!saved && JSON.stringify(saved.settings) !== JSON.stringify(draft);
  usePageActivity(onActivity,dirty,busy,"wintray",()=>void reload());
  function update(patch: Partial<Program>, id=selected) {
    if(busy||saved?.recoveryError)return;
    setDraft((current) =>
      current
        ? {
            ...current,
            managedApps: current.managedApps.map((a) =>
              a.id === id ? { ...a, ...patch } : a,
            ),
          }
        : current,
    );
  }
  function reorder(from: string, to: string) {
    if (busy || saved?.recoveryError || from === to) return;
    setDraft((current) => {
      if (!current) return current;
      const next = [...current.managedApps];
      const source = next.findIndex((a) => a.id === from),
        target = next.findIndex((a) => a.id === to);
      if (source < 0 || target < 0) return current;
      next.splice(target, 0, ...next.splice(source, 1));
      return { ...current, managedApps: next };
    });
  }
  const programDrop=usePathDrop({disabled:busy||!draft||!!saved?.recoveryError||view!=="programs",kind:"file",multiple:true,onPaths:paths=>add(paths),onError:setError});
  async function add(chosen?:string[]) {
    if(busy||!draft||saved?.recoveryError)return;
    setBusy(true);setError("");setMessage("");
    try {
      const paths = chosen??await pickPrograms();
      if (!paths.length) return;
      const infos:Awaited<ReturnType<typeof programInfo>>[]=new Array(paths.length);let cursor=0;
      // A large multi-file drop shares the existing Shell metadata service;
      // bound concurrent reads instead of starting a worker for every path.
      await Promise.all(Array.from({length:Math.min(4,paths.length)},async()=>{while(cursor<paths.length){const index=cursor++;infos[index]=await programInfo(paths[index]);}}));
      setMetadata(current=>({...current,...Object.fromEntries(paths.map((path,index)=>[path,infos[index]]))}));
      const entries = paths.map((path,index):Program=>{
      const exe = /\.exe$/i.test(path);
      return {
        id: crypto.randomUUID(),
        name: infos[index].name || path
          .split(/[\\/]/)
          .pop()!
          .replace(/\.[^.]+$/, ""),
        exePath: path,
        args: "",
        runOnStartup: true,
        collectTrayIcon: false,
        launchViaLogonTask: false,
        launchHiddenInBackground: !exe,
        trayBehavior: {
          autoMinimizeAndHideOnLaunch: exe,
          closeDelaySeconds: 0,
        },
        schedule: {
          enabled: false,
          startDelayMinutes: 0,
          autoExitMinutes: 30,
          frequencyEnabled: false,
          frequencyDays: 1,
          frequencyRuns: 1,
        },
      };
      });
      setDraft((s) =>
        s ? { ...s, managedApps: [...s.managedApps, ...entries] } : s,
      );
      setSelected(entries[entries.length-1].id);
    } catch (e) {
      setError(String(e));
    } finally {setBusy(false);}
  }
  async function replaceProgram(id:string) {
    if(busy)return;
    setBusy(true);setError("");setMessage("");
    try {
      const path=await pickProgram();if(!path)return;
      const info=await programInfo(path);setMetadata(current=>({...current,[path]:info}));
      // Preserve identity, arguments and scheduling. A different target cannot
      // inherit collected icons; scripts also cannot use executable-only modes.
      setDraft(current=>current?{...current,managedApps:current.managedApps.map(program=>{
        if(program.id!==id||program.exePath.replaceAll("/","\\").toLowerCase()===path.replaceAll("/","\\").toLowerCase())return program;
        const script=!/\.exe$/i.test(path);
        return {...program,exePath:path,name:info.name||path.split(/[\\/]/).pop()!.replace(/\.[^.]+$/,""),collectTrayIcon:false,
          launchViaLogonTask:script?false:program.launchViaLogonTask,
          launchHiddenInBackground:script?true:program.launchHiddenInBackground,
          trayBehavior:{...program.trayBehavior,autoMinimizeAndHideOnLaunch:script?false:program.trayBehavior.autoMinimizeAndHideOnLaunch}};
      })}:current);
    } catch(e){setError(String(e));}finally{setBusy(false);}
  }
  async function runJob(method: string, params: unknown) {
    if(method==="settings.save"&&(!validNumbers||!validNames))return;
    setBusy(true);
    setError("");
    setMessage("wintray.working");
    try {
      let job = await wintrayCall<Job>(method, params);
      // Completed configuration commits need no polling ID. Running worker
      // operations must retain the original ID until their result arrives.
      const id=job?.id;
      if(!job||(job.state==="running"&&(typeof id!=="string"||!id)))throw new Error(t("wintray.unknownResult"));
      const deadline = Date.now() + 5 * 60_000;
      while (job.state === "running") {
        if (Date.now() > deadline)
          throw new Error(t("wintray.pending"));
        await new Promise((resolve) => setTimeout(resolve, 250));
        job = await wintrayCall<Job>("jobs.get", { id });
        if(job?.id!==id)throw new Error(t("wintray.unknownResult"));
      }
      if (job.state !== "completed") throw new Error(job.error ?? t("wintray.failed"));
      setMessage(
        method === "launch"
          ? "wintray.launched"
          : method === "program.window" ? "wintray.windowChanged"
          : method === "program.stop" ? "wintray.stopped"
          : method === "recover"
            ? "wintray.recovered"
            : "common.saved",
      );
      await reload();
    } catch (e) {
      setError(String(e));
      setMessage("");
      await reload(true);
    } finally {
      setBusy(false);
    }
  }
  const navigationTabs=(<TabsList variant="default" className="max-w-full flex-wrap justify-start gap-1 group-data-horizontal/tabs:h-auto" aria-label={t("nav.wintray")}><TabsTrigger title={t("wintray.list")} value="programs"><IconLabel icon={List}>{t("wintray.list")}</IconLabel></TabsTrigger><TabsTrigger title={t("wintray.advancedStartup")} value="startup"><IconLabel icon={Power}>{t("wintray.advancedStartup")}</IconLabel></TabsTrigger></TabsList>);
  const unavailable=error
    ? <Notice actions={<Button disabled={busy} onClick={()=>void reload()}>{t("common.retry")}</Button>}>{error}</Notice>
    : <p role="status">{t("wintray.loading")}</p>;



  return (
    <section className="wintray-page" {...programDrop}>
      <Tabs value={view} onValueChange={value=>setView(String(value))}>
        {navigationTabs}
      {error && draft && (
        <Notice actions={<Button disabled={busy} onClick={() => void reload()}>{t("wintray.discard")}</Button>}>{error}</Notice>
      )}
      {disabled && <p className="muted">{t("wintray.disabled")}</p>}
      {!disabled && saved?.recoveryError && (
        <Button disabled={busy} onClick={() => void runJob("recover", null)}>{t("wintray.recover")}</Button>
      )}
      {!disabled && saved?.startupError && <Notice actions={<Button disabled={busy || dirty || !!saved.recoveryError} onClick={()=>void runJob("startup.retry",null)}>{t("wintray.retryStartup")}</Button>}>{saved.startupError}</Notice>}
        {draft&&<>
      {view==="programs"&&<div className="module-toolbar">
        <Button
          variant="primary"
          disabled={busy || !!saved?.recoveryError}
          onClick={() => void add()}
        >
          {t("wintray.add")}
        </Button>
      </div>}
      {!validNumbers&&<InlineError>{t("wintray.invalidNumbers",{sections:invalidTimingSections.join("、")})}</InlineError>}
      {!validNames&&<InlineError id="wintray-name-error">{t("wintray.nameRequired",{programs:invalidNames.join(", ")})}</InlineError>}
        </>}
        <TabsContent value="programs">{draft?<>
          <fieldset disabled={busy||!!saved?.recoveryError} className="plain-fieldset wintray-programs">
            <div className="wintray-workspace"><ResizablePanelGroup orientation="horizontal" className="wintray-panels border-t" disabled={busy}>
              <ResizablePanel defaultSize="40%" minSize="28%" style={{overflowX:"clip"}}>
                <ItemGroup data-page-scroll-target="" aria-label={t("wintray.list")} className="wintray-program-list gap-1 py-3 pr-3">
                  <div className="wintray-list-heading" aria-hidden="true"><span>{t("settingsDesign.programColumn")}</span><span>{t("settingsDesign.trayColumn")}</span><span>{t("settingsDesign.replaceColumn")}</span><span>{t("settingsDesign.orderColumn")}</span></div>
                  {!apps.length&&<p className="wintray-program-empty">{t("wintray.empty")}</p>}
                  {apps.map((a,index)=><Item key={a.id} role="listitem" aria-label={displayName(a)} aria-current={selected===a.id?"true":undefined} tabIndex={busy?-1:0} onClick={()=>{if(!busy)setSelected(a.id);}} onKeyDown={event=>{if(event.target===event.currentTarget&&!busy&&(event.key==="Enter"||event.key===" ")){event.preventDefault();setSelected(a.id);}}} variant={selected===a.id?"muted":"default"} className="wintray-program-row cursor-pointer" draggable={!busy} onDragStart={()=>setDragged(a.id)} onDragEnd={()=>setDragged("")} onDragOver={event=>event.preventDefault()} onDrop={event=>{if(dragged){event.preventDefault();event.stopPropagation();if(!busy)reorder(dragged,a.id);setDragged("");}}}>
                    {metadata[a.exePath]?.icon?<img src={`data:image/png;base64,${metadata[a.exePath].icon}`} alt="" className="size-8 shrink-0"/>:<AppWindow aria-hidden="true" className="size-8 shrink-0 text-muted-foreground"/>}
                    <span className="wintray-program-name" title={displayName(a)}>{displayName(a)}</span>
                    <div className="wintray-row-controls" onClick={event=>event.stopPropagation()}>
                      <div className="wintray-collect-control flex items-center gap-2" title={t("wintray.collect")}><Switch id={"collect-"+a.id} checked={a.collectTrayIcon} disabled={busy||!!saved?.recoveryError||!/\.exe$/i.test(a.exePath)} onCheckedChange={value=>update({collectTrayIcon:value},a.id)}/><Label className="wintray-collect-label" htmlFor={"collect-"+a.id}>{t("wintray.collect")}</Label></div>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="icon" title={t("wintray.replaceProgram")} aria-label={t("wintray.replaceNamed",{name:displayName(a)})} onClick={()=>void replaceProgram(a.id)}><FolderOpen size={16}/></Button>
                        <Select value={String(index+1)} disabled={busy||!!saved?.recoveryError} items={apps.map((_,position)=>({value:String(position+1),label:String(position+1)}))} onValueChange={value=>{const target=Number(value)-1;if(value&&Number.isInteger(target)&&apps[target])reorder(a.id,apps[target].id);}}>
                          <SelectTrigger className="w-20" aria-label={t("wintray.sortPosition",{name:displayName(a)})}><SelectValue/></SelectTrigger>
                          <SelectContent>{apps.map((_,position)=><SelectItem key={position} value={String(position+1)}>{String(position+1)}</SelectItem>)}</SelectContent>
                        </Select>
                      </div>
                    </div>
                  </Item>)}
                </ItemGroup>
              </ResizablePanel>
              <ResizableHandle withHandle aria-label={t("wintray.resizePanels")}/>
              <ResizablePanel minSize="30%" style={{overflowX:"clip"}}>
                <div data-page-scroll-target="" className="wintray-program-detail min-w-0 space-y-4 p-5">
                  {active?<>
                    <div className="wintray-detail-header"><div className="min-w-0 flex-1"><h2 className="break-words">{displayName(active)}</h2><p className="truncate text-sm text-muted-foreground" title={active.exePath}>{active.exePath}</p></div>
                    <div className="flex flex-wrap gap-2" role="group" aria-label={t("wintray.programActions")}>
                      <WinTrayProgramActions key={active.id} id={active.id} name={displayName(active)} disabled={disabled||dirty||!!saved?.recoveryError} busy={busy} onRun={runJob}/>
                      <Button onClick={()=>{const next=apps.filter(a=>a.id!==active.id);setDraft({...draft,managedApps:next});setSelected(next[0]?.id??"");}}>{t("wintray.remove")}</Button>
                    </div>
                    </div>
                    <WinTrayProgramSettings key={active.id} program={active} blocked={busy||!!saved?.recoveryError} update={update}/>
                  </>:<p className="text-muted-foreground">{t("wintray.select")}</p>}
                </div>
              </ResizablePanel>
            </ResizablePanelGroup></div>
          </fieldset>
        </>:unavailable}</TabsContent>
        <TabsContent data-page-scroll-target="" value="startup">{draft?<>
          <fieldset disabled={busy||!!saved?.recoveryError} className="plain-fieldset settings-section-stack">
            <SettingsSection icon={Rocket} title={t("settingsDesign.launchBehavior")}><div className="settings-switch-row"><Label htmlFor="wintray-exit">{t("wintray.exitWhenDone")}</Label><Switch id="wintray-exit" checked={draft.exitAfterManagedAppsCompleted} disabled={busy||!!saved?.recoveryError} onCheckedChange={value=>setDraft({...draft,exitAfterManagedAppsCompleted:value})}/></div>
            </SettingsSection>
            <SettingsSection icon={Timer} title={t("settingsDesign.waitTimes")}><div className="settings-field-grid">
              <NumberField label={t("wintray.interval")} value={draft.startupIntervalSeconds} max={120} onChange={value=>setDraft({...draft,startupIntervalSeconds:value})}/>
              <NumberField label={t("wintray.windowWait")} value={draft.closeWindowRetrySeconds} max={120} onChange={value=>setDraft({...draft,closeWindowRetrySeconds:value})}/>
            </div></SettingsSection>
            <WinTrayMaintenance disabled={busy||dirty||!!saved?.recoveryError} onBusyChange={setBusy} onChanged={async()=>{await reload();onChanged();}}/>
          </fieldset>
        </>:unavailable}</TabsContent>

      </Tabs>
      {/* The shared save transaction stays outside both scrolling panes. */}
      {draft&&<SettingsSaveBar undo={{disabled:busy||!dirty,onClick:()=>void reload()}}>
        <Button icon={Save} variant="primary"
          disabled={busy || !dirty || !validNumbers || !validNames || !!saved?.recoveryError}
          onClick={() => void runJob("settings.save", {revision: saved!.revision, settings: draft})}>
          {t("wintray.save")}
        </Button>
        <span role="status">
          {busy ? (message?t(message):"") : dirty ? t("wintray.dirty") : message?t(message):""}
        </span>
      </SettingsSaveBar>}
    </section>
  );
}
